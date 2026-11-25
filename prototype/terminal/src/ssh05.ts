/**
 * ssh05 UI: OpenTUI + IpcClient + TabStore against the local supervisor.
 *
 * It never opens SSH directly. All sessions, input, resize and status go
 * through ssh05d over the typed IPC contract.
 *
 * Run:
 *   SSH05_SOCKET=/run/user/1000/ssh05/control.sock bun run src/ssh05.ts
 *
 * Producto de Dinámicas Electrónicas · soporte: dinamicas.electronicas@gmail.com
 */

import {
  BoxRenderable,
  EmbeddedTerminalRenderable,
  type KeyEvent,
  type MouseEvent,
  MouseButton,
  RGBA,
  StyledText,
  TextRenderable,
  type TextChunk,
  bg as bgColor,
  bold,
  createCliRenderer,
  fg,
} from "@opentui/core"
import { appendFileSync, existsSync } from "node:fs"
import { join } from "node:path"
import type {
  FrpEndpoint,
  FrpProxy,
  HostProfile,
  HostKeyChallenge,
  IpcEvent,
  SessionId,
  SessionInfo,
  SessionPhase,
  SessionStatus,
  TabId,
  TransferInfo,
  TabRecord,
  TabState,
  Workspace,
} from "../../../contracts/ts/index.ts"
import { loadKeybindings, matchesAction } from "./config/keybindings.ts"
import { RPC_CODE_HOST_KEY_CHANGED, RPC_CODE_HOST_KEY_UNKNOWN } from "../../../contracts/ts/index.ts"
import { exportDiagnostics } from "./diagnostics/export.ts"
import { isValidAlias, normalizeAlias } from "./hosts/alias.ts"
import { envValue, resolveSocketPath, SUPERVISOR } from "./identity.ts"
import {
  loadPreferences,
  nextPanelWidth,
  savePreferences,
  type Preferences,
} from "./preferences/store.ts"
import { normalizeHex } from "./theme/color.ts"
import { IpcClient, IpcRemoteError } from "./ipc/client.ts"
import { InMemoryTabStore } from "./tabs/store.ts"
import { phaseIcon, phaseLabel, renderTabBar, renderTabBarStyled, tabBarHitTest } from "./tabs/tabbar.ts"
import { listWorkspaceNames, loadWorkspace, safeWorkspaceName, saveWorkspace } from "./workspaces/store.ts"

let preferences: Preferences = loadPreferences()
let hostPanelWidth = preferences.hostPanelWidth
const MAX_SESSIONS = 16
const UI_COLORS = {
  accent: "#7FB4FF",
  accentSoft: "#4E6A93",
  selectedBg: "#2D3B52",
  selectedFg: "#FFFFFF",
  dim: "#7D879A",
  danger: "#FF7A7A",
  warning: "#F2C94C",
  success: "#7BD88F",
  panelBg: RGBA.fromInts(16, 19, 26),
  frame: RGBA.fromInts(48, 58, 76),
} as const
const MAX_PENDING_BYTES = 1 << 20

const debugFile = envValue("UI_DEBUG")
const debug = (message: string) => {
  if (debugFile) {
    try {
      appendFileSync(debugFile, `${new Date().toISOString()} ${message}\n`)
    } catch {
      // Debug logging must never break the UI.
    }
  }
}

const loadedBindings = loadKeybindings()
const keybindings = loadedBindings.bindings
for (const error of loadedBindings.errors) debug(error)

const socketPath = resolveSocketPath()

const cliArgs = process.argv.slice(2)
let currentWorkspace = envValue("WORKSPACE_NAME") || "default"
for (let index = 0; index < cliArgs.length; index++) {
  if (cliArgs[index] === "--workspace" && cliArgs[index + 1]) {
    currentWorkspace = safeWorkspaceName(cliArgs[++index])
  }
}
currentWorkspace = safeWorkspaceName(currentWorkspace)

const renderer = await createCliRenderer({
  exitOnCtrlC: false,
  useMouse: true,
  enableMouseMovement: true,
})

const root = new BoxRenderable(renderer, {
  id: "root",
  width: "100%",
  height: "100%",
  flexDirection: "column",
  backgroundColor: RGBA.fromInts(15, 17, 22),
})

const tabBar = new TextRenderable(renderer, {
  id: "tabbar",
  content: " ssh05 · Dinámicas Electrónicas",
  fg: RGBA.fromInts(225, 230, 240),
  bg: RGBA.fromInts(38, 44, 58),
  height: 1,
})

const body = new BoxRenderable(renderer, {
  id: "body",
  width: "100%",
  flexGrow: 1,
  flexShrink: 1,
  flexDirection: "row",
})

const hostPanel = new BoxRenderable(renderer, {
  id: "hosts",
  width: hostPanelWidth,
  height: "100%",
  flexDirection: "column",
  backgroundColor: RGBA.fromInts(22, 25, 33),
})

const hostTitle = new TextRenderable(renderer, {
  id: "host-title",
  content: " Hosts",
  fg: RGBA.fromInts(120, 190, 255),
  bg: RGBA.fromInts(28, 33, 44),
  height: 1,
})

const hostQuery = new TextRenderable(renderer, {
  id: "host-query",
  content: " ↑↓←→ seleccionar",
  fg: RGBA.fromInts(125, 135, 150),
  bg: RGBA.fromInts(22, 25, 33),
  height: 1,
})

const hostList = new TextRenderable(renderer, {
  id: "host-list",
  content: "",
  fg: RGBA.fromInts(205, 212, 225),
  bg: RGBA.fromInts(22, 25, 33),
  flexGrow: 1,
  flexShrink: 1,
  wrapMode: "none",
})

const hostHint = new TextRenderable(renderer, {
  id: "host-hint",
  content: " Enter conectar",
  fg: RGBA.fromInts(110, 120, 135),
  bg: RGBA.fromInts(22, 25, 33),
  height: 1,
})

hostPanel.add(hostTitle)
hostPanel.add(hostQuery)
hostPanel.add(hostList)
hostPanel.add(hostHint)

hostList.onMouseDown = (event) => {
  const row = event.y - hostList.y
  if (commandMode) {
    const hit = commandLineMap.find((item) => item.line === row)
    if (!hit) return
    event.stopPropagation()
    commandMode = false
    commandInput = ""
    hit.command.run("")
    uiDirty()
    return
  }
  const hit = hostLineMap.find((item) => item.line === row)
  if (!hit) return
  hostFocused = true
  cursor = hit.index
  if (event.button === MouseButton.RIGHT) {
    event.stopPropagation()
    openHostMenu(hit.host)
    return
  }
  if (event.button !== MouseButton.LEFT) return
  event.stopPropagation()
  const now = Date.now()
  if (lastHostClick.id === hit.host.id && now - lastHostClick.at < 450) {
    lastHostClick = { id: "", at: 0 }
    void connectHost(hit.host)
    return
  }
  lastHostClick = { id: hit.host.id, at: now }
  uiDirty()
}

hostList.onMouseMove = (event) => {
  const row = event.y - hostList.y
  const hit = hostLineMap.find((item) => item.line === row)
  if (!hit) return
  if (cursor === hit.index && hostFocused) return
  cursor = hit.index
  hostFocused = true
  uiDirty()
}

hostList.onMouseScroll = (event) => {
  if (event.scroll?.direction === "up") cursor = Math.max(0, cursor - 3)
  else if (event.scroll?.direction === "down") {
    cursor = Math.min(Math.max(0, filteredHosts().length - 1), cursor + 3)
  }
  uiDirty()
}

const sessionPane = new BoxRenderable(renderer, {
  id: "session-pane",
  flexGrow: 1,
  flexShrink: 1,
  height: "100%",
  flexDirection: "column",
  border: true,
  borderStyle: "rounded",
  borderColor: UI_COLORS.frame,
  backgroundColor: UI_COLORS.panelBg,
})

const sessionHeader = new BoxRenderable(renderer, {
  id: "session-header",
  width: "100%",
  height: 1,
  flexDirection: "row",
  backgroundColor: RGBA.fromInts(24, 29, 40),
})

const sessionTitle = new TextRenderable(renderer, {
  id: "session-title",
  content: " Sin sesión · selecciona un host",
  fg: RGBA.fromInts(160, 172, 190),
  bg: RGBA.fromInts(24, 29, 40),
  height: 1,
  flexGrow: 1,
  flexShrink: 1,
  minWidth: 0,
  wrapMode: "none",
})

const sessionClose = new TextRenderable(renderer, {
  id: "session-close",
  content: " × ",
  fg: RGBA.fromInts(255, 122, 122),
  bg: RGBA.fromInts(24, 29, 40),
  width: 3,
  height: 1,
  visible: false,
  onMouseDown: (event) => {
    if (event.button === MouseButton.RIGHT) {
      openCloseDialog()
      return
    }
    if (event.button === MouseButton.LEFT) void closeActive("close-view")
  },
  onMouseOver: () => {
    sessionClose.bg = RGBA.fromInts(190, 62, 62)
    sessionClose.fg = RGBA.fromInts(255, 255, 255)
    uiDirty()
  },
  onMouseOut: () => {
    sessionClose.bg = RGBA.fromInts(24, 29, 40)
    sessionClose.fg = RGBA.fromInts(255, 122, 122)
    uiDirty()
  },
})

sessionHeader.add(sessionTitle)
sessionHeader.add(sessionClose)

sessionTitle.onMouseDown = () => {
  const active = store.active()
  if (active) focusTerminal()
  else focusHosts()
}

const terminalArea = new BoxRenderable(renderer, {
  id: "terminal-area",
  flexGrow: 1,
  flexShrink: 1,
  backgroundColor: UI_COLORS.panelBg,
})

sessionPane.add(sessionHeader)
sessionPane.add(terminalArea)
body.add(hostPanel)
body.add(sessionPane)

const status = new TextRenderable(renderer, {
  id: "status",
  content: "",
  fg: RGBA.fromInts(150, 160, 175),
  bg: RGBA.fromInts(24, 27, 35),
  height: 1,
})

root.add(tabBar)
root.add(body)
root.add(status)
renderer.root.add(root)
applyTabPosition()
hostPanel.width = hostPanelWidth
renderer.start()

tabBar.onMouseDown = (event) => {
  const hit = tabBarHitTest(
    { tabs: store.list(), activeId: store.active()?.id, width: renderer.width },
    event.x - tabBar.x,
  )
  if (!hit) return
  event.stopPropagation()
  if (hit.kind === "hidden-left") {
    cycle(-1)
    return
  }
  if (hit.kind === "hidden-right") {
    cycle(1)
    return
  }
  activate(hit.id)
  if (event.button === MouseButton.RIGHT || event.button === MouseButton.MIDDLE) {
    openCloseDialog()
  } else {
    focusTerminal()
  }
}

tabBar.onMouseScroll = (event) => {
  if (event.scroll?.direction === "up") cycle(-1)
  else if (event.scroll?.direction === "down") cycle(1)
}

const workspaceOverlay = new TextRenderable(renderer, {
  id: "workspace-overlay",
  position: "absolute",
  top: 0,
  left: 0,
  width: "100%",
  height: "100%",
  visible: false,
  zIndex: 100,
  content: "",
  fg: RGBA.fromInts(215, 222, 235),
  bg: RGBA.fromInts(18, 21, 28),
})

renderer.root.add(workspaceOverlay)

const paletteOverlay = new TextRenderable(renderer, {
  id: "palette-overlay",
  position: "absolute",
  top: 0,
  left: 0,
  width: "100%",
  height: "100%",
  visible: false,
  zIndex: 101,
  content: "",
  fg: RGBA.fromInts(219, 226, 238),
  bg: RGBA.fromInts(16, 19, 26),
})

renderer.root.add(paletteOverlay)

const closeOverlay = new TextRenderable(renderer, {
  id: "close-overlay",
  position: "absolute",
  top: 0,
  left: 0,
  width: "100%",
  height: "100%",
  visible: false,
  zIndex: 102,
  content: "",
  fg: RGBA.fromInts(219, 226, 238),
  bg: RGBA.fromInts(16, 19, 26),
})

renderer.root.add(closeOverlay)

const hostKeyOverlay = new TextRenderable(renderer, {
  id: "host-key-overlay",
  position: "absolute",
  top: 0,
  left: 0,
  width: "100%",
  height: "100%",
  visible: false,
  zIndex: 135,
  content: "",
  fg: RGBA.fromInts(255, 224, 170),
  bg: RGBA.fromInts(24, 20, 14),
})

renderer.root.add(hostKeyOverlay)

const frpOverlay = new TextRenderable(renderer, {
  id: "frp-overlay",
  position: "absolute",
  top: 0,
  left: 0,
  width: "100%",
  height: "100%",
  visible: false,
  zIndex: 103,
  content: "",
  fg: RGBA.fromInts(219, 226, 238),
  bg: RGBA.fromInts(16, 19, 26),
})

renderer.root.add(frpOverlay)

const labelsOverlay = new TextRenderable(renderer, {
  id: "labels-overlay",
  position: "absolute",
  top: 0,
  left: 0,
  width: "100%",
  height: "100%",
  visible: false,
  zIndex: 104,
  content: "",
  fg: RGBA.fromInts(219, 226, 238),
  bg: RGBA.fromInts(16, 19, 26),
})

renderer.root.add(labelsOverlay)

const diagOverlay = new TextRenderable(renderer, {
  id: "diag-overlay",
  position: "absolute",
  top: 0,
  left: 0,
  width: "100%",
  height: "100%",
  visible: false,
  zIndex: 105,
  content: "",
  fg: RGBA.fromInts(219, 226, 238),
  bg: RGBA.fromInts(16, 19, 26),
})

renderer.root.add(diagOverlay)

const prefsOverlay = new TextRenderable(renderer, {
  id: "prefs-overlay",
  position: "absolute",
  top: 0,
  left: 0,
  width: "100%",
  height: "100%",
  visible: false,
  zIndex: 106,
  content: "",
  fg: RGBA.fromInts(219, 226, 238),
  bg: RGBA.fromInts(16, 19, 26),
})

renderer.root.add(prefsOverlay)

const transferOverlay = new TextRenderable(renderer, {
  id: "transfer-overlay",
  position: "absolute",
  top: 0,
  left: 0,
  width: "100%",
  height: "100%",
  visible: false,
  zIndex: 107,
  content: "",
  fg: RGBA.fromInts(219, 226, 238),
  bg: RGBA.fromInts(16, 19, 26),
})

renderer.root.add(transferOverlay)

const welcomeOverlay = new TextRenderable(renderer, {
  id: "welcome-overlay",
  position: "absolute",
  top: 0,
  left: 0,
  width: "100%",
  height: "100%",
  visible: false,
  zIndex: 140,
  content: "",
  fg: RGBA.fromInts(219, 226, 238),
  bg: RGBA.fromInts(13, 16, 22),
})

renderer.root.add(welcomeOverlay)

const hostMenuOverlay = new TextRenderable(renderer, {
  id: "host-menu-overlay",
  position: "absolute",
  top: 0,
  left: 0,
  width: "100%",
  height: "100%",
  visible: false,
  zIndex: 130,
  content: "",
  fg: RGBA.fromInts(219, 226, 238),
  bg: RGBA.fromInts(15, 18, 25),
})

renderer.root.add(hostMenuOverlay)

const deleteConfirmOverlay = new TextRenderable(renderer, {
  id: "delete-confirm-overlay",
  position: "absolute",
  top: 0,
  left: 0,
  width: "100%",
  height: "100%",
  visible: false,
  zIndex: 131,
  content: "",
  fg: RGBA.fromInts(219, 226, 238),
  bg: RGBA.fromInts(15, 18, 25),
})

renderer.root.add(deleteConfirmOverlay)

const nmapOverlay = new TextRenderable(renderer, {
  id: "nmap-overlay",
  position: "absolute",
  top: 0,
  left: 0,
  width: "100%",
  height: "100%",
  visible: false,
  zIndex: 108,
  content: "",
  fg: RGBA.fromInts(255, 210, 160),
  bg: RGBA.fromInts(30, 20, 14),
})

renderer.root.add(nmapOverlay)

const helpOverlay = new TextRenderable(renderer, {
  id: "help-overlay",
  position: "absolute",
  top: 0,
  left: 0,
  width: "100%",
  height: "100%",
  visible: false,
  zIndex: 109,
  content: "",
  fg: RGBA.fromInts(219, 226, 238),
  bg: RGBA.fromInts(16, 19, 26),
})

renderer.root.add(helpOverlay)

const newHostOverlay = new TextRenderable(renderer, {
  id: "new-host-overlay",
  position: "absolute",
  top: 0,
  left: 0,
  width: "100%",
  height: "100%",
  visible: false,
  zIndex: 111,
  content: "",
  fg: RGBA.fromInts(219, 226, 238),
  bg: RGBA.fromInts(16, 19, 26),
})

renderer.root.add(newHostOverlay)

const pendingPlaceholder = new TextRenderable(renderer, {
  id: "pending-view",
  position: "absolute",
  top: 0,
  left: 0,
  width: "100%",
  height: "100%",
  visible: false,
  content: "",
  fg: RGBA.fromInts(170, 180, 195),
  bg: RGBA.fromInts(15, 17, 22),
})
terminalArea.add(pendingPlaceholder)

bindOverlayMouse("workspace", workspaceOverlay)
bindOverlayMouse("palette", paletteOverlay)
bindOverlayMouse("close", closeOverlay)
bindOverlayMouse("hostkey", hostKeyOverlay)
bindOverlayMouse("frp", frpOverlay)
bindOverlayMouse("labels", labelsOverlay)
bindOverlayMouse("diag", diagOverlay)
bindOverlayMouse("prefs", prefsOverlay)
bindOverlayMouse("transfer", transferOverlay)
bindOverlayMouse("welcome", welcomeOverlay)
bindOverlayMouse("hostmenu", hostMenuOverlay)
bindOverlayMouse("delete", deleteConfirmOverlay)
bindOverlayMouse("nmap", nmapOverlay)
bindOverlayMouse("help", helpOverlay)
bindOverlayMouse("newhost", newHostOverlay)

type SessionView = {
  sessionId: SessionId
  host: HostProfile
  phase: SessionPhase
  route: string
  terminal: EmbeddedTerminalRenderable
  resizeTimer?: ReturnType<typeof setTimeout>
  pendingResize?: { cols: number; rows: number }
}

const store = new InMemoryTabStore()
const sessions = new Map<SessionId, SessionView>()
const pendingData = new Map<SessionId, Uint8Array[]>()
const pendingStatus = new Map<SessionId, SessionStatus>()
const pendingHosts = new Map<TabId, HostProfile>()

let client: IpcClient | undefined
let hosts: HostProfile[] = []
let conflicts = 0
let hostFocused = true
let cursor = 0
let scroll = 0
let query = ""
let tmuxMode = false
let quitting = false
let errorText = ""
let transientText = ""
let restoringWorkspace = false
let saveTimer: ReturnType<typeof setTimeout> | undefined
let workspacePickerVisible = false
let workspacePickerCursor = 0
let workspaceNames: string[] = []
let workspaceNewMode = false
let workspaceInput = ""
let paletteVisible = false
let paletteCursor = 0
let closeDialogVisible = false
let closeDialogCursor = 0
let hostKeyDialog: {
  challenge: HostKeyChallenge
  host: HostProfile
  options: { tmuxSession?: string; tmuxCreate?: boolean; replaceTabId?: TabId }
} | null = null
let hostKeyCursor = 0
let newHostVisible = false
let newHostForm: {
  mode: "new" | "edit"
  editingAlias?: string
  error: string
  field: number
  alias: string
  hostname: string
  port: string
  username: string
  identityFile: string
  password: string
} | null = null
let helpVisible = false
let nmapConfirmVisible = false
let nmapPorts = "22,80,443"
let transferVisible = false
let transferForm: {
  direction: "upload" | "download"
  protocol: "sftp" | "scp"
  localPath: string
  remotePath: string
  recursive: boolean
  field: number
} | null = null
let transfers: TransferInfo[] = []
let transferCursor = 0
let transferPoll: ReturnType<typeof setInterval> | undefined
let preferencesVisible = false
let preferencesCursor = 0
let diagVisible = false
let diagContent = ""
let labelsEditing = false
let labelsHostID = ""
let labelsInput = ""
let vaultAgent = "sin agente"
let vaultState = "sin vault"
let frpVisible = false
let frpCursor = 0
let frpProxies: FrpProxy[] = []
let frpDeleteArmed = ""
let frpEndpoints: FrpEndpoint[] = []
let frpForm: {
  field: number
  name: string
  type: string
  servicePort: string
  remotePort: string
  bindPort: string
} | null = null
let welcomeVisible = envValue("NO_WELCOME") !== "1"
let caretOn = true
let caretPos = 0
let commandMode = false
let commandInput = ""
let hostMenuVisible = false
let hostMenuCursor = 0
let hostMenuAlias = ""
let deleteConfirmVisible = false
let deleteConfirmAlias = ""
let deleteConfirmCursor = 0
let actionBarFocused = false
let actionBarCursor = 0
let actionBarHover = -1
let actionsMenuVisible = false
let actionsMenuCursor = 0
let actionsMenuScroll = 0
let hostLineMap: { line: number; host: HostProfile; index: number }[] = []
let commandLineMap: { line: number; command: SlashCommand }[] = []
let lastHostClick = { id: "", at: 0 }

type OverlayRegion = {
  line: number
  fromCol?: number
  toCol?: number
  onHover?: () => void
  onClick: (event: MouseEvent) => void
}

const overlayMouseRegions = new Map<string, OverlayRegion[]>()

const actionBar = new BoxRenderable(renderer, {
  id: "action-bar",
  width: "100%",
  height: 1,
  flexDirection: "row",
  backgroundColor: RGBA.fromInts(30, 36, 48),
})

const actionBarButtons: { node: TextRenderable; run: () => void }[] = []

function addActionButton(label: string, run: () => void): void {
  const index = actionBarButtons.length
  const node = new TextRenderable(renderer, {
    id: `action-${index}`,
    content: ` ${label} `,
    width: label.length + 2,
    height: 1,
    fg: RGBA.fromInts(215, 222, 235),
    bg: RGBA.fromInts(38, 44, 58),
    onMouseDown: () => {
      actionBarFocused = true
      actionBarCursor = index
      run()
      actionBarFocused = false
      actionBarHover = -1
      uiDirty()
    },
    onMouseOver: () => {
      actionBarHover = index
      uiDirty()
    },
    onMouseOut: () => {
      if (actionBarHover === index) actionBarHover = -1
      uiDirty()
    },
  })
  actionBar.add(node)
  actionBarButtons.push({ node, run })
}

addActionButton("Menú", () => openActionsMenu())
addActionButton("Hosts", () => focusHosts())
addActionButton("Nuevo", () => openNewHost())
addActionButton("WS", () => openWorkspacePicker())
addActionButton("Rutas", () => void probeCurrentHost())
addActionButton("Diag", () => void diagnoseCurrentHost())
addActionButton("Transferir", () => void openTransfers())
addActionButton("FRP", () => void openFRP())
addActionButton("Prefs", () => openPreferences())
addActionButton("Ayuda", () => { helpVisible = true })
addActionButton("Salir", () => void quit())

const actionsMenuItems: { label: string; run: () => void }[] = [
  { label: "Ir a hosts", run: focusHosts },
  { label: "Nuevo host", run: openNewHost },
  { label: "Importar/recargar KeePass", run: () => void importHosts() },
  { label: "Alternar tmux", run: () => { tmuxMode = !tmuxMode; uiDirty() } },
  { label: "Cerrar pestaña", run: openCloseDialog },
  { label: "Cambiar workspace", run: openWorkspacePicker },
  { label: "Probar ruta", run: () => void probeCurrentHost() },
  { label: "Diagnosticar host", run: () => void diagnoseCurrentHost() },
  { label: "Escanear con Nmap", run: openNmapConfirm },
  { label: "Información de red", run: () => void showNetworkInfo() },
  { label: "Exportar diagnóstico saneado", run: () => void exportDiagnosticsReport() },
  { label: "Preferencias", run: () => openPreferences() },
  { label: "Transferencias", run: () => void openTransfers() },
  { label: "Subir archivo", run: () => void openTransferQuick("upload") },
  { label: "Descargar archivo", run: () => void openTransferQuick("download") },
  { label: "FRP", run: () => void openFRP() },
  { label: "Favorito del host", run: () => void toggleFavorite() },
  { label: "Color del host", run: () => void cycleHostColor() },
  { label: "Etiquetas del host", run: openLabelsEditor },
  { label: "Ayuda", run: () => { helpVisible = true } },
  { label: "Salir", run: () => void quit() },
]

const actionsMenuBox = new BoxRenderable(renderer, {
  id: "actions-menu",
  position: "absolute",
  top: 3,
  left: 3,
  width: 76,
  height: Math.min(actionsMenuItems.length + 4, 32),
  flexDirection: "column",
  visible: false,
  zIndex: 112,
  backgroundColor: RGBA.fromInts(18, 22, 30),
})

const actionsMenuTitle = new TextRenderable(renderer, {
  id: "actions-title",
  content: " Acciones",
  width: "100%",
  height: 1,
  fg: RGBA.fromInts(120, 190, 255),
  bg: RGBA.fromInts(28, 33, 44),
})
actionsMenuBox.add(actionsMenuTitle)

const actionsMenuRows: TextRenderable[] = []
for (let index = 0; index < actionsMenuItems.length; index++) {
  const row = new TextRenderable(renderer, {
    id: `actions-row-${index}`,
    content: "",
    width: "100%",
    height: 1,
    onMouseDown: () => {
      actionsMenuCursor = index
      executeActionsMenu()
    },
    onMouseOver: () => {
      actionsMenuCursor = index
      uiDirty()
    },
  })
  actionsMenuBox.add(row)
  actionsMenuRows.push(row)
}
actionsMenuBox.onMouseScroll = (event) => {
  if (event.scroll?.direction === "up") actionsMenuCursor = Math.max(0, actionsMenuCursor - 1)
  else if (event.scroll?.direction === "down") {
    actionsMenuCursor = Math.min(actionsMenuItems.length - 1, actionsMenuCursor + 1)
  }
  uiDirty()
}
renderer.root.add(actionsMenuBox)
root.add(actionBar, 1)

const paletteCommands = [
  "Ir a hosts",
  "Nuevo host",
  "Importar/recargar KeePass",
  "Alternar tmux",
  "Cerrar pestaña activa",
  "Cambiar workspace",
  "Probar ruta del host",
  "Diagnosticar host",
  "Escanear con Nmap",
  "Información de red",
  "Exportar diagnóstico saneado",
  "Preferencias",
  "Transferencias",
  "FRP",
  "Salir de ssh05",
  "Ayuda",
]

const paletteDescriptions = [
  "Enfoca la lista de hosts y sus atajos",
  "Crea un host y lo guarda en ssh_config",
  "Importa hosts desde KeePass (KDBX)",
  "Activa o desactiva tmux remoto antes de conectar",
  "Elige cómo cerrar la pestaña o desconectar SSH",
  "Cambia el workspace activo o crea uno nuevo",
  "Comprueba DNS, TCP y latencia del host",
  "DNS, puerto, banner y TLS del host seleccionado",
  "Escaneo optativo con confirmación explícita",
  "ASN/RDAP y país orientativo del host",
  "Guarda un informe saneado sin secretos",
  "Ancho, densidad y posición de pestañas",
  "Subir o descargar con SCP/SFTP",
  "Endpoints y proxies FRP",
  "Cierra ssh05; las sesiones tmux siguen vivas",
  "Muestra todos los atajos de teclado y ratón",
]

type SlashCommand = { name: string; help: string; run: (arg: string) => void }

const slashCommands: SlashCommand[] = [
  { name: "connect", help: "Crear y conectar un host", run: (arg) => openNewHost(arg) },
  { name: "new", help: "Crear un host", run: (arg) => openNewHost(arg) },
  { name: "hosts", help: "Ir a la lista de hosts", run: () => focusHosts() },
  { name: "workspaces", help: "Cambiar de workspace", run: () => openWorkspacePicker() },
  { name: "transfers", help: "Ver transferencias", run: () => void openTransfers() },
  { name: "frp", help: "Abrir el panel FRP", run: () => void openFRP() },
  { name: "preferences", help: "Ajustar preferencias", run: () => openPreferences() },
  { name: "help", help: "Mostrar la ayuda", run: () => { helpVisible = true } },
  { name: "quit", help: "Salir de ssh05", run: () => void quit() },
]

function slashMatches(input: string): SlashCommand[] {
  const query = input.trim().split(/\s+/)[0]?.toLowerCase() ?? ""
  if (!query) return slashCommands
  return slashCommands.filter((command) => command.name.startsWith(query))
}

function renderHostMenu(): void {
  if (!hostMenuVisible) {
    hostMenuOverlay.visible = false
    return
  }
  hostMenuOverlay.visible = true
  const items = [
    { label: "Conectar", icon: "▶" },
    { label: "Editar", icon: "✎" },
    { label: "Eliminar", icon: "✕" },
  ]
  const lines = ["", `  Host · ${hostMenuAlias}`, ""]
  const regions: OverlayRegion[] = []
  items.forEach((item, index) => {
    const line = lines.length
    lines.push(`  ${index === hostMenuCursor ? "▸" : " "} ${item.icon} ${item.label}`)
    regions.push({
      line,
      onHover: () => {
        hostMenuCursor = index
      },
      onClick: () => executeHostMenuItem(index),
    })
  })
  lines.push("", "  ↑↓←→ elegir · Enter aceptar · Esc volver · clic para elegir")
  setOverlayRegions("hostmenu", regions)
  hostMenuOverlay.content = styledOverlay(lines, 3 + hostMenuCursor, [1])
}

function executeHostMenuItem(index: number): void {
  const host = hosts.find((item) => item.alias === hostMenuAlias)
  hostMenuVisible = false
  if (!host) {
    uiDirty()
    return
  }
  if (index === 0) void connectHost(host)
  else if (index === 1) openHostEditor(host)
  else openDeleteConfirm(host)
}

function renderDeleteConfirm(): void {
  if (!deleteConfirmVisible) {
    deleteConfirmOverlay.visible = false
    return
  }
  deleteConfirmOverlay.visible = true
  const lines = [
    "",
    "  Eliminar host",
    "",
    `  ¿Eliminar “${deleteConfirmAlias}” de ssh_config?`,
    "",
    `  ${deleteConfirmCursor === 0 ? "▸" : " "} [ Sí, eliminar ]`,
    `  ${deleteConfirmCursor === 1 ? "▸" : " "} [ Cancelar ]`,
    "",
    "  ↑↓ elegir · Enter confirmar · Esc cancelar · clic para elegir",
  ]
  setOverlayRegions("delete", [
    {
      line: 5,
      onHover: () => {
        deleteConfirmCursor = 0
      },
      onClick: () => void deleteHostAlias(deleteConfirmAlias),
    },
    {
      line: 6,
      onHover: () => {
        deleteConfirmCursor = 1
      },
      onClick: () => {
        deleteConfirmVisible = false
        uiDirty()
      },
    },
  ])
  deleteConfirmOverlay.content = styledOverlay(lines, 5 + deleteConfirmCursor, [1])
}

function editingFieldActive(): boolean {
  return (
    newHostVisible ||
    commandMode ||
    labelsEditing ||
    (transferVisible && transferForm !== null) ||
    (frpVisible && frpForm !== null) ||
    (workspacePickerVisible && workspaceNewMode) ||
    nmapConfirmVisible
  )
}

setInterval(() => {
  if (!editingFieldActive()) return
  caretOn = !caretOn
  uiDirty()
}, 500)

function renderWelcome(): void {
  if (!welcomeVisible) {
    welcomeOverlay.visible = false
    return
  }
  welcomeOverlay.visible = true
  const lines = [
    "",
    "  ssh05 · Dinámicas Electrónicas",
    "",
    "  Bienvenido",
    "",
    "  ↑/↓        mover por los hosts",
    "  ←/→        barra superior / acciones del host",
    "  Enter      conectar / abrir el menú seleccionado",
    "  /          comandos: /connect crea un host",
    "  Ctrl+N     nuevo host     Ctrl+K  paleta     Ctrl+P  workspaces",
    "  F1 ayuda   F2 favorito    F3 color           F4 etiquetas",
    "  Ratón      clic en hosts, pestañas y botones; rueda para desplazar",
    "  F10 salir",
    "",
  ]
  const regions = addActionLine(lines, "Acciones", [
    { label: "Continuar", run: () => { welcomeVisible = false; uiDirty() } },
  ])
  lines.push("", "  Pulsa Enter o haz clic para continuar")
  setOverlayRegions("welcome", regions)
  welcomeOverlay.content = styledOverlay(lines, -1, [1, 3])
}


function truncate(value: string, max: number): string {
  if (value.length <= max) return value
  return `${value.slice(0, Math.max(0, max - 1))}…`
}

function hostTarget(host: HostProfile): string {
  const user = host.target.username ? `${host.target.username}@` : ""
  return `${user}${host.target.hostname}:${host.target.port}`
}

function tmuxSessionName(alias: string): string {
  const safe = alias.replace(/[^A-Za-z0-9_-]+/g, "-").replace(/^-+|-+$/g, "")
  return `ssh05-${safe || "host"}`
}

function isDockerHost(host: HostProfile): boolean {
  if ((host.sourceRefs ?? []).some((ref) => ref.kind === "docker")) return true
  return (host.labels ?? []).some((label) => label.toLowerCase() === "docker")
}

function groupedHosts(list: HostProfile[]): HostProfile[] {
  return [...list].sort((a, b) => Number(isDockerHost(a)) - Number(isDockerHost(b)))
}

function filteredHosts(): HostProfile[] {
  const needle = query.trim().toLowerCase()
  if (!needle) return groupedHosts(hosts)
  return groupedHosts(hosts.filter((host) =>
    [
      host.alias,
      host.target.hostname,
      host.target.username ?? "",
      String(host.target.port),
      (host.labels ?? []).join(" "),
      host.favorite ? "favorite" : "",
    ]
      .join(" ")
      .toLowerCase()
      .includes(needle),
  ))
}

function sessionForHost(host: HostProfile): SessionView | undefined {
  for (const view of sessions.values()) {
    if (view.host.id === host.id) return view
  }
  return undefined
}

function pendingHostFor(host: HostProfile): HostProfile | undefined {
  for (const pending of pendingHosts.values()) {
    if (pending.id === host.id) return pending
  }
  return undefined
}

function renderHosts(): void {
  const all = filteredHosts()
  hostLineMap = []
  commandLineMap = []
  hostTitle.content = ` Hosts (${all.length}${query ? `/${hosts.length}` : ""})${conflicts > 0 ? ` !${conflicts}` : ""}`

  if (commandMode) {
    hostQuery.content = ` /${withCaret(commandInput, true)}`
    hostHint.content = " /connect crea un host · Enter ejecutar · Esc cancelar"
    const matches = slashMatches(commandInput)
    const lines = ["", "  Comandos", ""]
    commandLineMap = []
    if (matches.length === 0) {
      lines.push("", `  Sin comandos para /${commandInput}`)
    } else {
      for (const command of matches) {
        commandLineMap.push({ line: lines.length, command })
        lines.push(`  ▸ /${command.name}  ${command.help}`)
      }
    }
    lines.push("", "  Enter ejecutar · Esc cancelar · clic para ejecutar")
    hostList.content = styledLines(lines, -1, hostPanelWidth, [1])
    return
  }
  commandLineMap = []

  hostQuery.content = query ? ` /${truncate(query, 24)}` : " ↑↓ hosts · ←→ menú"
  hostHint.content =
    hosts.length === 0
      ? " Ctrl+N nuevo host · /connect · Ctrl+I KeePass"
      : " Enter conectar · ←/→ menú · / comandos · Ctrl+N nuevo"

  if (hosts.length === 0) {
    hostList.content =
      `\n No hay hosts disponibles.\n\n` +
      ` ${SUPERVISOR}: ${socketPath}\n\n` +
      " Ctrl+N o /connect crea un host; también lee ~/.ssh/config.\n" +
      " Contenedores Docker: añade --docker al servicio\n" +
      "   (systemctl --user edit ssh05d.service)\n"
    return
  }

  const rowHeight = preferences.density === "comfortable" ? 2 : 1
  const rows = Math.max(
    1,
    Math.floor((hostList.height || Math.max(1, renderer.height - 7)) / rowHeight),
  )
  if (all.length === 0) {
    hostList.content = `\n Sin resultados para “${query}”`
    return
  }
  cursor = Math.max(0, Math.min(cursor, all.length - 1))

  type DisplayRow =
    | { kind: "header"; label: string }
    | { kind: "host"; host: HostProfile; index: number }
  const display: DisplayRow[] = []
  let lastDocker: boolean | undefined
  const dockerCount = all.filter(isDockerHost).length
  const sshCount = all.length - dockerCount
  for (let index = 0; index < all.length; index++) {
    const host = all[index]
    const docker = isDockerHost(host)
    if (docker !== lastDocker) {
      const label = docker ? `Contenedores (${dockerCount})` : `Hosts SSH (${sshCount})`
      display.push({ kind: "header", label })
      lastDocker = docker
    }
    display.push({ kind: "host", host, index })
  }

  const cursorRow = Math.max(
    0,
    display.findIndex((row) => row.kind === "host" && row.index === cursor),
  )
  if (cursorRow < scroll) scroll = cursorRow
  if (cursorRow >= scroll + rows) scroll = cursorRow - rows + 1
  scroll = Math.max(0, Math.min(scroll, Math.max(0, display.length - rows)))

  const lines: string[] = []
  const headerLines: number[] = []
  hostLineMap = []
  for (let row = scroll; row < Math.min(display.length, scroll + rows); row++) {
    const entry = display[row]
    if (entry.kind === "header") {
      headerLines.push(lines.length)
      lines.push(`  ── ${entry.label} ──`)
    } else {
      const host = entry.host
      const session = sessionForHost(host)
      const tab = session ? store.list().find((item) => item.sessionId === session.sessionId) : undefined
      const pending = !session && pendingHostFor(host) !== undefined
      const state = tab ? phaseIcon(tab.phase) : pending ? "○" : " "
      const favorite = host.favorite ? "★" : " "
      const group = host.labels && host.labels.length > 0 ? ` (${host.labels[0]})` : ""
      const label = `${favorite}${host.alias}${group}`
      hostLineMap.push({ line: lines.length, host, index: entry.index })
      lines.push(`${entry.index === cursor ? "▸" : " "} ${state} ${truncate(label, hostPanelWidth - 4)}`)
    }
    if (preferences.density === "comfortable") lines.push("")
  }
  const selectedLine = hostLineMap.find((item) => item.index === cursor)?.line ?? -1
  hostList.content = styledLines(lines, selectedLine, hostPanelWidth, headerLines)
}

function renderTabs(): void {
  tabBar.content = renderTabBarStyled({
    tabs: store.list(),
    activeId: store.active()?.id,
    width: renderer.width,
  })
}

function renderSessionHeader(): void {
  const active = store.active()
  const view = active ? sessions.get(active.sessionId) : undefined
  const available = Math.max(10, renderer.width - hostPanelWidth - 8)
  if (!active) {
    sessionTitle.content = truncate(" Sin sesión · Enter conecta · Ctrl+N nuevo host", available)
    sessionTitle.fg = RGBA.fromInts(150, 160, 175)
    sessionClose.visible = false
    sessionPane.borderColor = UI_COLORS.frame
    return
  }
  sessionClose.visible = true
  if (view) {
    const tmux = active.tmux ? ` · tmux ${active.tmux.sessionName}` : ""
    sessionTitle.content = truncate(
      ` ${phaseIcon(active.phase)} ${active.title} · ${hostTarget(view.host)} · ${view.route}${tmux}`,
      available,
    )
    sessionTitle.fg =
      active.phase === "connected"
        ? RGBA.fromInts(123, 216, 143)
        : active.phase === "failed"
          ? RGBA.fromInts(255, 122, 122)
          : RGBA.fromInts(242, 201, 76)
    sessionPane.borderColor =
      active.phase === "connected"
        ? RGBA.fromInts(66, 120, 88)
        : active.phase === "failed"
          ? RGBA.fromInts(150, 70, 70)
          : UI_COLORS.frame
    return
  }
  sessionTitle.content = truncate(` ○ ${active.title} · sesión pendiente · Enter reconectar`, available)
  sessionTitle.fg = RGBA.fromInts(160, 172, 190)
  sessionPane.borderColor = UI_COLORS.frame
}

function renderStatus(): void {
  if (errorText) {
    status.content = ` ${errorText}`
    return
  }
  if (transientText) {
    status.content = ` ${transientText}`
    return
  }
  if (hostFocused) {
    status.content =
      ` Hosts · ↑↓←→ Enter conectar · Ctrl+N nuevo · F2 favorito · F3 color · F4 etiquetas` +
      ` · tmux:${tmuxMode ? "on" : "off"} · F10 salir`
    return
  }
  const active = store.active()
  const view = active ? sessions.get(active.sessionId) : undefined
  if (!active || !view) {
    if (active && pendingHosts.has(active.id)) {
      status.content =
        ` Pendiente · ${active.title} · Enter reconectar` +
        `${active.tmux ? " (tmux)" : ""} · Ctrl+W cerrar · F10 salir`
      return
    }
    status.content = ` Sin sesiones · Ctrl+T hosts · tmux:${tmuxMode ? "on" : "off"} · F10 salir`
    return
  }
  status.content =
    ` SSH ${phaseIcon(active.phase)} ${phaseLabel(active.phase)} · ${active.title} · ${hostTarget(view.host)}` +
    ` · ${view.route} · tmux:${active.tmux ? "on" : "off"} · Ctrl+Tab cambiar · Ctrl+W cerrar`
}

function uiDirty(): void {
  renderHosts()
  renderTabs()
  renderSessionHeader()
  renderStatus()
  renderWorkspacePicker()
  renderPalette()
  renderCloseDialog()
  renderHostKeyDialog()
  renderFRP()
  renderLabelsEditor()
  renderDiagnostics()
  renderPreferences()
  renderActionBar()
  renderActionsMenu()
  renderTransfers()
  renderNmapConfirm()
  renderHelp()
  renderNewHost()
  renderHostMenu()
  renderDeleteConfirm()
  renderWelcome()
  renderer.requestRender()
}

type CloseChoice = {
  label: string
  intent: "close-view" | "disconnect-ssh" | "detach-remote-tmux"
}

function closeChoices(): CloseChoice[] {
  const choices: CloseChoice[] = [
    { label: "Cerrar vista (desconecta SSH)", intent: "close-view" },
    { label: "Desconectar SSH", intent: "disconnect-ssh" },
  ]
  if (store.active()?.tmux) {
    choices.push({
      label: "Desacoplar tmux (mantener sesión remota)",
      intent: "detach-remote-tmux",
    })
  }
  return choices
}

function padLine(line: string, width = Math.max(20, renderer.width)): string {
  return line + " ".repeat(Math.max(0, width - line.length))
}

function isHintLine(trimmed: string): boolean {
  return (
    trimmed.startsWith("↑") ||
    trimmed.startsWith("Enter") ||
    trimmed.startsWith("Esc") ||
    trimmed.startsWith("Ctrl") ||
    trimmed.startsWith("Tab") ||
    trimmed.startsWith("Espacio") ||
    trimmed.startsWith("Pulsa") ||
    trimmed.startsWith("Acciones:") ||
    trimmed.startsWith("Solo úsalo") ||
    trimmed.startsWith("Verifica la huella") ||
    trimmed.startsWith("La contraseña se guarda") ||
    trimmed.startsWith("Se guardará") ||
    trimmed.startsWith("StrictHostKeyChecking") ||
    trimmed.startsWith("Ajusta ssh_config")
  )
}

function styledOverlay(lines: string[], selectedLine = -1, titleLines: number[] = [1]): StyledText {
  return styledLines(lines, selectedLine, Math.max(20, renderer.width), titleLines)
}

function styledLines(lines: string[], selectedLine: number, width: number, titleLines: number[] = []): StyledText {
  const chunks: TextChunk[] = []
  lines.forEach((line, index) => {
    const padded = padLine(line, width)
    const trimmed = line.trim()
    let chunk: TextChunk
    if (index === selectedLine) {
      chunk = bgColor(UI_COLORS.selectedBg)(fg(UI_COLORS.selectedFg)(bold(padded)))
    } else if (titleLines.includes(index)) {
      chunk = fg(UI_COLORS.accent)(bold(padded))
    } else if (trimmed.startsWith("⚠") || trimmed.includes("IT IS POSSIBLE") || trimmed.startsWith("NO SE PUDO")) {
      chunk = fg(UI_COLORS.danger)(padded)
    } else if (isHintLine(trimmed)) {
      chunk = fg(UI_COLORS.dim)(padded)
    } else if (
      trimmed.startsWith("FRP ·") ||
      trimmed.startsWith("Hosts SSH") ||
      trimmed.startsWith("Contenedores") ||
      trimmed.startsWith("Transferencias")
    ) {
      chunk = fg(UI_COLORS.accentSoft)(bold(padded))
    } else {
      chunk = { __isChunk: true, text: padded }
    }
    chunks.push(chunk)
    if (index < lines.length - 1) chunks.push({ __isChunk: true, text: "\n" })
  })
  return new StyledText(chunks)
}

type OverlayAction = { label: string; run: () => void }

function addActionLine(lines: string[], prefix: string, actions: OverlayAction[]): OverlayRegion[] {
  const line = lines.length
  let text = `  ${prefix}  `
  const regions: OverlayRegion[] = []
  for (const action of actions) {
    const label = `[ ${action.label} ]`
    const start = text.length
    text += label + "  "
    regions.push({
      line,
      fromCol: start,
      toCol: start + label.length - 1,
      onClick: (event) => {
        event.stopPropagation()
        action.run()
      },
    })
  }
  lines.push(text)
  return regions
}

function setOverlayRegions(name: string, regions: OverlayRegion[]): void {
  overlayMouseRegions.set(name, regions)
}

function findOverlayRegion(name: string, row: number, col: number): OverlayRegion | undefined {
  return overlayMouseRegions.get(name)?.find(
    (region) =>
      region.line === row &&
      (region.fromCol === undefined || (col >= region.fromCol && col <= (region.toCol ?? region.fromCol))),
  )
}

function bindOverlayMouse(name: string, overlay: TextRenderable): void {
  let lastHoverRow = -2
  overlay.onMouseDown = (event) => {
    if (event.button !== MouseButton.LEFT) return
    const region = findOverlayRegion(name, event.y - overlay.y, event.x - overlay.x)
    if (!region) return
    event.stopPropagation()
    region.onClick(event)
  }
  overlay.onMouseMove = (event) => {
    const row = event.y - overlay.y
    if (row === lastHoverRow) return
    lastHoverRow = row
    const region = findOverlayRegion(name, row, event.x - overlay.x)
    if (region?.onHover) {
      region.onHover()
      uiDirty()
    }
  }
}

function renderCloseDialog(): void {
  if (!closeDialogVisible) {
    closeOverlay.visible = false
    return
  }
  closeOverlay.visible = true
  const choices = closeChoices()
  closeDialogCursor = Math.min(closeDialogCursor, choices.length - 1)
  const lines = ["", "  Cerrar conexión", ""]
  const regions: OverlayRegion[] = []
  choices.forEach((choice, index) => {
    const line = lines.length
    lines.push(`  ${index === closeDialogCursor ? "▸" : " "} ${choice.label}`)
    regions.push({
      line,
      onHover: () => {
        closeDialogCursor = index
      },
      onClick: () => executeCloseChoice(choice),
    })
  })
  lines.push("", "  ↑↓ elegir · Enter confirmar · Esc cancelar · clic para elegir")
  setOverlayRegions("close", regions)
  closeOverlay.content = styledOverlay(lines, 3 + closeDialogCursor, [1])
}

function executeCloseChoice(choice: CloseChoice): void {
  closeDialogVisible = false
  void closeActive(choice.intent)
}

function openCloseDialog(): void {
  const active = store.active()
  if (!active) return
  if (!sessions.has(active.sessionId)) {
    void closeActive("close-view")
    return
  }
  closeDialogCursor = 0
  closeDialogVisible = true
  uiDirty()
}

function closeCloseDialog(): void {
  closeDialogVisible = false
  uiDirty()
}

function executeCloseDialog(): void {
  const choice = closeChoices()[closeDialogCursor]
  if (!choice) return
  executeCloseChoice(choice)
}

function handleCloseDialogKey(key: KeyEvent): void {
  const name = key.name.toLowerCase()
  if (name === "up" || name === "left") closeDialogCursor = Math.max(0, closeDialogCursor - 1)
  else if (name === "down" || name === "right") {
    closeDialogCursor = Math.min(closeChoices().length - 1, closeDialogCursor + 1)
  } else if (name === "home") closeDialogCursor = 0
  else if (name === "end") closeDialogCursor = closeChoices().length - 1
  else if (name === "escape") {
    closeCloseDialog()
    return
  } else if (name === "return" || name === "enter") {
    executeCloseDialog()
    return
  }
  uiDirty()
}

function renderHostKeyDialog(): void {
  if (!hostKeyDialog) {
    hostKeyOverlay.visible = false
    return
  }
  hostKeyOverlay.visible = true
  const { challenge, host } = hostKeyDialog
  const lines = [
    "",
    "  Host desconocido",
    "",
    `  ${host.alias} · ${challenge.host || hostTarget(host)}`,
    `  Tipo de clave: ${challenge.keyType}`,
    `  Huella SHA-256: ${challenge.fingerprint}`,
  ]
  if (challenge.knownHostsPath) lines.push(`  Se guardará en: ${challenge.knownHostsPath}`)
  lines.push("")
  const regions: OverlayRegion[] = []
  if (challenge.canTrust) {
    lines.push("  Verifica la huella con el administrador del servidor.", "")
    const choiceStart = lines.length
    lines.push(`  ${hostKeyCursor === 0 ? "▸" : " "} [ Aceptar y conectar ]`)
    lines.push(`  ${hostKeyCursor === 1 ? "▸" : " "} [ Cancelar ]`)
    regions.push(
      {
        line: choiceStart,
        onHover: () => {
          hostKeyCursor = 0
        },
        onClick: () => void acceptHostKey(),
      },
      {
        line: choiceStart + 1,
        onHover: () => {
          hostKeyCursor = 1
        },
        onClick: closeHostKeyDialog,
      },
    )
  } else {
    lines.push("  StrictHostKeyChecking=yes impide añadir esta clave.")
    lines.push("  Ajusta ssh_config o añádela manualmente.", "")
    const choiceStart = lines.length
    lines.push(`  ${hostKeyCursor === 0 ? "▸" : " "} [ Cerrar ]`)
    regions.push({ line: choiceStart, onClick: closeHostKeyDialog })
  }
  lines.push("", "  ↑↓ elegir · Enter confirmar · Esc cancelar · clic para elegir")
  setOverlayRegions("hostkey", regions)
  const choiceLine = lines.findIndex((line) => line.includes("[ Aceptar") || line.includes("[ Cerrar ]"))
  hostKeyOverlay.content = styledOverlay(lines, choiceLine + hostKeyCursor, [1])
}

function closeHostKeyDialog(): void {
  hostKeyDialog = null
  hostKeyCursor = 0
  transientText = "Conexión cancelada"
  uiDirty()
}

async function acceptHostKey(): Promise<void> {
  const pending = hostKeyDialog
  if (!pending || !client) return
  hostKeyDialog = null
  hostKeyCursor = 0
  transientText = `Añadiendo host key de ${pending.host.alias}…`
  uiDirty()
  try {
    await client.call(
      "hosts.trustKey",
      { hostProfileId: pending.host.id, fingerprint: pending.challenge.fingerprint },
      { timeoutMs: 20_000 },
    )
    transientText = `Host key aceptada; conectando ${pending.host.alias}…`
    uiDirty()
    await connectHost(pending.host, pending.options)
  } catch (error) {
    transientText = `No se pudo confiar en ${pending.host.alias}: ${String(error)}`
    uiDirty()
  }
}

function handleHostKeyDialogKey(key: KeyEvent): void {
  const name = key.name.toLowerCase()
  if (!hostKeyDialog) return
  const choices = hostKeyDialog.challenge.canTrust ? 2 : 1
  if (name === "up" || name === "left") hostKeyCursor = Math.max(0, hostKeyCursor - 1)
  else if (name === "down" || name === "right") hostKeyCursor = Math.min(choices - 1, hostKeyCursor + 1)
  else if (name === "home") hostKeyCursor = 0
  else if (name === "end") hostKeyCursor = choices - 1
  else if (name === "escape") {
    closeHostKeyDialog()
    return
  } else if (name === "return" || name === "enter") {
    if (hostKeyDialog.challenge.canTrust && hostKeyCursor === 0) {
      void acceptHostKey()
    } else {
      closeHostKeyDialog()
    }
    return
  }
  uiDirty()
}

function renderPalette(): void {
  if (!paletteVisible) {
    paletteOverlay.visible = false
    return
  }
  paletteOverlay.visible = true
  const lines = ["", "  Paleta de Comandos", ""]
  const regions: OverlayRegion[] = []
  for (let index = 0; index < paletteCommands.length; index++) {
    const selected = index === paletteCursor
    const line = lines.length
    lines.push(`  ${selected ? "▸" : " "} ${paletteCommands[index]}`)
    regions.push({
      line,
      onHover: () => {
        paletteCursor = index
      },
      onClick: () => executePalette(index),
    })
  }
  lines.push(
    "",
    `  ${paletteDescriptions[paletteCursor] ?? ""}`,
    "",
    "  ↑↓←→ mover · Enter ejecutar · Esc cerrar · clic para elegir",
  )
  setOverlayRegions("palette", regions)
  paletteOverlay.content = styledOverlay(lines, 3 + paletteCursor, [1])
}

function openPalette(): void {
  if (workspacePickerVisible) closeWorkspacePicker()
  paletteVisible = true
  paletteCursor = 0
  uiDirty()
}

function closePalette(): void {
  paletteVisible = false
  uiDirty()
}

function executePalette(index: number): void {
  closePalette()
  switch (index) {
    case 0:
      focusHosts()
      break
    case 1:
      openNewHost()
      break
    case 2:
      void importHosts()
      break
    case 3:
      tmuxMode = !tmuxMode
      uiDirty()
      break
    case 4:
      openCloseDialog()
      break
    case 5:
      openWorkspacePicker()
      break
    case 6:
      void probeCurrentHost()
      break
    case 7:
      void diagnoseCurrentHost()
      break
    case 8:
      openNmapConfirm()
      break
    case 9:
      void showNetworkInfo()
      break
    case 10:
      void exportDiagnosticsReport()
      break
    case 11:
      openPreferences()
      break
    case 12:
      void openTransfers()
      break
    case 13:
      void openFRP()
      break
    case 14:
      void quit()
      break
    case 15:
      helpVisible = true
      break
  }
}

function handlePaletteKey(key: KeyEvent): void {
  const name = key.name.toLowerCase()
  if (name === "up" || name === "left") paletteCursor = Math.max(0, paletteCursor - 1)
  else if (name === "down" || name === "right") paletteCursor = Math.min(paletteCommands.length - 1, paletteCursor + 1)
  else if (name === "pageup") paletteCursor = Math.max(0, paletteCursor - 6)
  else if (name === "pagedown") paletteCursor = Math.min(paletteCommands.length - 1, paletteCursor + 6)
  else if (name === "home") paletteCursor = 0
  else if (name === "end") paletteCursor = paletteCommands.length - 1
  else if (name === "escape") {
    closePalette()
    return
  } else if (name === "return" || name === "enter") {
    executePalette(paletteCursor)
    return
  }
  uiDirty()
}

function snapshotWorkspace(): Workspace {
  const tabs: TabRecord[] = store.list().map((tab, index) => {
    const view = sessions.get(tab.sessionId)
    const pending = pendingHosts.get(tab.id)
    return {
      id: tab.id,
      title: tab.title,
      hostProfileId: view?.host.id ?? pending?.id ?? tab.title,
      sessionId: view ? tab.sessionId : undefined,
      tmux: tab.tmux,
      color: tab.color,
      order: index,
    }
  })
  return {
    id: currentWorkspace,
    name: currentWorkspace,
    tabs,
    preferences: { activeTabId: store.active()?.id ?? "" },
  }
}

function scheduleWorkspaceSave(): void {
  if (restoringWorkspace || quitting) return
  if (saveTimer) clearTimeout(saveTimer)
  saveTimer = setTimeout(() => {
    saveTimer = undefined
    try {
      saveWorkspace(snapshotWorkspace(), currentWorkspace)
    } catch (error) {
      debug(`workspace save: ${String(error)}`)
    }
  }, 150)
}

async function restoreWorkspace(
  liveSessions: SessionInfo[],
  workspace = loadWorkspace(currentWorkspace),
): Promise<void> {
  const records = workspace ? [...workspace.tabs].sort((a, b) => a.order - b.order) : []
  const used = new Set<SessionId>()
  restoringWorkspace = true
  try {
    for (const record of records) {
      const host = hosts.find(
        (item) => item.id === record.hostProfileId || item.alias === record.hostProfileId,
      )
      if (!host) continue

      const existing = record.sessionId ? sessions.get(record.sessionId) : undefined
      if (existing) {
        const id = record.id || existing.sessionId
        if (!store.list().some((tab) => tab.id === id)) {
          const tab = tabForSession(
            existing.sessionId,
            host,
            existing.phase,
            record.tmux?.sessionName,
          )
          store.add({ ...tab, id })
        }
        used.add(existing.sessionId)
        continue
      }

      const live = liveSessions.find(
        (info) =>
          !used.has(info.sessionId) &&
          (info.sessionId === record.sessionId || info.profileId === host.id),
      )
      if (live) {
        try {
          used.add(live.sessionId)
          await attachExisting(live, host, record.id)
          continue
        } catch (error) {
          debug(`attach ${live.sessionId}: ${String(error)}`)
        }
      }

      const id = record.id || `restored-${record.hostProfileId}`
      if (store.list().some((tab) => tab.id === id)) continue
      store.add({
        id,
        sessionId: id,
        title: record.title || host.alias,
        phase: "disconnected",
        color: record.color || host.color,
        tmux: record.tmux,
      })
      pendingHosts.set(id, host)
    }

    const preferred = workspace?.preferences?.activeTabId
    const activeId =
      preferred && store.list().some((tab) => tab.id === preferred)
        ? preferred
        : store.list()[0]?.id
    if (activeId) activate(activeId)
    else focusHosts()
  } finally {
    restoringWorkspace = false
  }
  scheduleWorkspaceSave()
  uiDirty()
}

function focusHosts(): void {
  hostFocused = true
  uiDirty()
}

function focusTerminal(): void {
  const active = store.active()
  if (!active) {
    focusHosts()
    return
  }
  hostFocused = false
  sessions.get(active.sessionId)?.terminal.focus()
  uiDirty()
}

function activate(tabId: TabId): void {
  store.activate(tabId)
  const active = store.active()
  if (!active) {
    focusHosts()
    return
  }
  const view = sessions.get(active.sessionId)
  for (const [sessionId, item] of sessions) {
    item.terminal.visible = sessionId === active.sessionId
  }
  if (view) {
    pendingPlaceholder.visible = false
    hostFocused = false
    view.terminal.focus()
  } else if (pendingHosts.has(active.id)) {
    hostFocused = false
    pendingPlaceholder.visible = true
    pendingPlaceholder.content = pendingMessage(active)
  } else {
    pendingPlaceholder.visible = false
    focusHosts()
    return
  }
  uiDirty()
}

function pendingMessage(tab: TabState): string {
  const host = pendingHosts.get(tab.id)
  const target = host ? hostTarget(host) : "host desconocido"
  return (
    `\n  ${tab.title}\n\n` +
    `  Sesión pendiente de restaurar (${target}).\n\n` +
    `  Enter: reconectar${tab.tmux ? ` y reenganchar tmux ${tab.tmux.sessionName}` : ""}\n` +
    `  Ctrl+W: quitar la pestaña del workspace\n`
  )
}

function cycle(delta: number): void {
  const tabs = store.list()
  if (tabs.length === 0) return
  const active = store.active()
  const index = active ? tabs.findIndex((tab) => tab.id === active.id) : 0
  const next = tabs[(index + delta + tabs.length) % tabs.length]
  activate(next.id)
}

function flushPending(sessionId: SessionId, view: SessionView): void {
  const buffered = pendingData.get(sessionId)
  if (buffered) {
    for (const chunk of buffered) view.terminal.write(chunk)
    pendingData.delete(sessionId)
  }
  const status = pendingStatus.get(sessionId)
  if (status) {
    const tab = store.list().find((item) => item.sessionId === sessionId)
    if (tab) {
      store.update(tab.id, { phase: status.phase })
      if (status.tmuxSession === "") store.update(tab.id, { tmux: undefined })
    }
    pendingStatus.delete(sessionId)
  }
}

function createTerminalView(
  sessionId: SessionId,
  host: HostProfile,
  tmuxSession?: string,
  phase: SessionPhase = "connected",
  route = "SSH directo",
): SessionView {
  const cols = Math.max(20, Math.floor(terminalArea.width) || renderer.width - hostPanelWidth - 2)
  const rows = Math.max(5, (terminalArea.height || renderer.height - 3) - 1)
  const terminal = new EmbeddedTerminalRenderable(renderer, {
    id: `terminal-${sessionId}`,
    position: "absolute",
    top: 0,
    left: 0,
    width: "100%",
    height: "100%",
    visible: false,
    cols,
    rows,
    maxScrollback: 20_000,
    selectable: true,
    onData: (data, source) => {
      if ((source === "input" || source === "response") && client) {
        client.sendBinary(sessionId, data)
      }
    },
    onTerminalResize: (newCols, newRows) => {
      if (newCols > 0 && newRows > 0) scheduleResize(sessionId, newCols, newRows)
    },
  })
  terminalArea.add(terminal)
  const view: SessionView = { sessionId, host, phase, route, terminal }
  sessions.set(sessionId, view)
  return view
}

function tabForSession(sessionId: SessionId, host: HostProfile, phase: SessionPhase, tmuxSession?: string): TabState {
  return {
    id: sessionId,
    sessionId,
    title: host.alias,
    phase,
    color: host.color,
    tmux: tmuxSession ? { hostProfileId: host.id, sessionName: tmuxSession, verified: true } : undefined,
  }
}

async function connectHost(
  host: HostProfile,
  options: { tmuxSession?: string; tmuxCreate?: boolean; replaceTabId?: TabId } = {},
): Promise<void> {
  if (!client || quitting) return
  if (sessions.size >= MAX_SESSIONS) {
    transientText = `Máximo de ${MAX_SESSIONS} sesiones alcanzado`
    uiDirty()
    return
  }
  transientText = ""
  errorText = ""
  const wantNewTmux = options.tmuxSession === undefined && options.tmuxCreate !== false && tmuxMode
  const tmuxSession = options.tmuxSession ?? (wantNewTmux ? tmuxSessionName(host.alias) : undefined)
  const tmuxCreate = options.tmuxSession === undefined ? wantNewTmux : options.tmuxCreate === true
  let route = "SSH directo"
  try {
    const plan = await client.call(
      "routes.plan",
      { hostProfileId: host.id },
      { timeoutMs: 3000 },
    )
    route = plan.reason || plan.selected || route
  } catch (error) {
    debug(`routes.plan ${host.id}: ${String(error)}`)
  }
  try {
    const opened = await client.call(
      "sessions.open",
      { hostProfileId: host.id, tmuxSession, tmuxCreate },
      { timeoutMs: 20_000 },
    )
    if (options.replaceTabId) {
      pendingHosts.delete(options.replaceTabId)
      store.remove(options.replaceTabId)
    }
    const attachedTmux = opened.tmuxSession || undefined
    if (tmuxSession && !attachedTmux) {
      transientText = `tmux ${tmuxSession} no existía; se abrió una shell nueva`
    }
    const view = createTerminalView(opened.sessionId, host, attachedTmux, "connected", route)
    const tab = tabForSession(opened.sessionId, host, "connected", attachedTmux)
    store.add(tab)
    activate(tab.id)
    flushPending(opened.sessionId, view)
    scheduleResize(opened.sessionId, view.terminal.width || 80, view.terminal.height || 24)
  } catch (error) {
    if (error instanceof IpcRemoteError && error.code === RPC_CODE_HOST_KEY_UNKNOWN) {
      const data = error.data as HostKeyChallenge | undefined
      if (data && typeof data.fingerprint === "string" && typeof data.keyType === "string") {
        hostKeyDialog = { challenge: { ...data, canTrust: data.canTrust !== false }, host, options }
        hostKeyCursor = 0
        transientText = `Host key nueva para ${host.alias}`
        uiDirty()
        return
      }
    }
    if (error instanceof IpcRemoteError && error.code === RPC_CODE_HOST_KEY_CHANGED) {
      transientText = `La host key de ${host.alias} ha cambiado. Revisa known_hosts antes de continuar.`
      uiDirty()
      return
    }
    transientText = `No se pudo abrir ${host.alias}: ${String(error)}`
    uiDirty()
  }
}

async function attachExisting(
  info: SessionInfo,
  host: HostProfile,
  replaceTabId?: TabId,
): Promise<void> {
  if (!client || sessions.has(info.sessionId)) return
  await client.call("sessions.attach", { sessionId: info.sessionId }, { timeoutMs: 5000 })
  if (replaceTabId) {
    pendingHosts.delete(replaceTabId)
    store.remove(replaceTabId)
  }
  const view = createTerminalView(info.sessionId, host, info.tmuxSession, info.phase, "SSH reenganchado")
  const tab = tabForSession(info.sessionId, host, info.phase, info.tmuxSession)
  store.add(tab)
  activate(tab.id)
  flushPending(info.sessionId, view)
  scheduleResize(info.sessionId, view.terminal.width || 80, view.terminal.height || 24)
}

function scheduleResize(sessionId: SessionId, cols: number, rows: number): void {
  const view = sessions.get(sessionId)
  if (!view || !client) return
  view.pendingResize = { cols, rows }
  if (view.resizeTimer) clearTimeout(view.resizeTimer)
  view.resizeTimer = setTimeout(() => {
    const size = view.pendingResize
    view.pendingResize = undefined
    if (!size || !client) return
    client
      .call(
        "sessions.resize",
        { sessionId, cols: size.cols, rows: size.rows },
        { timeoutMs: 3000 },
      )
      .catch((error) => debug(`resize ${sessionId}: ${String(error)}`))
  }, 80)
}

async function closeActive(intent: "close-view" | "disconnect-ssh" | "detach-remote-tmux" = "close-view"): Promise<void> {
  const active = store.active()
  if (!active) return
  const view = sessions.get(active.sessionId)
  if (view && client) {
    try {
      await client.call(
        "sessions.close",
        { sessionId: active.sessionId, intent },
        { timeoutMs: 5000 },
      )
    } catch (error) {
      transientText = `Cierre: ${String(error)}`
    }
  }

  sessions.delete(active.sessionId)
  pendingHosts.delete(active.id)
  pendingData.delete(active.sessionId)
  pendingStatus.delete(active.sessionId)
  if (view) {
    if (view.resizeTimer) clearTimeout(view.resizeTimer)
    try {
      terminalArea.remove(view.terminal)
      view.terminal.destroy()
    } catch {
      // already gone
    }
  }
  store.remove(active.id)
  pendingPlaceholder.visible = false
  const next = store.active()
  if (next) activate(next.id)
  else focusHosts()
  uiDirty()
}

function moveActiveTab(delta: number): void {
  const tabs = store.list()
  const active = store.active()
  if (!active || tabs.length < 2) return
  const index = tabs.findIndex((tab) => tab.id === active.id)
  const target = Math.min(tabs.length - 1, Math.max(0, index + delta))
  if (target === index) return
  store.move(active.id, target)
  uiDirty()
}

function reconnectActive(): void {
  const active = store.active()
  if (!active) return
  const host = pendingHosts.get(active.id)
  if (!host) return
  void connectHost(host, {
    replaceTabId: active.id,
    tmuxSession: active.tmux?.sessionName,
    tmuxCreate: false,
  })
}

async function quit(): Promise<void> {
  if (quitting) return
  if (saveTimer) {
    clearTimeout(saveTimer)
    saveTimer = undefined
  }
  try {
    saveWorkspace(snapshotWorkspace(), currentWorkspace)
  } catch (error) {
    debug(`workspace save on quit: ${String(error)}`)
  }
  quitting = true
  const tabs = [...store.list()].filter((tab) => sessions.has(tab.sessionId))
  if (client) {
    await Promise.all(
      tabs.map((tab) =>
        client!
          .call("sessions.close", { sessionId: tab.sessionId, intent: "disconnect-ssh" }, { timeoutMs: 1000 })
          .catch(() => undefined),
      ),
    )
    client.close()
  }
  renderer.destroy()
  process.exit(0)
}

const HOST_COLORS = ["", "#5878CC", "#27AE60", "#F2C94C", "#EB5757", "#9B51E0", "#56CCF2"]

async function applyHostOverride(
  hostID: string,
  patch: { labels?: string[]; favorite?: boolean; color?: string },
): Promise<boolean> {
  if (!client) return false
  try {
    await client.call(
      "hosts.overrides.set",
      { hostProfileId: hostID, ...patch },
      { timeoutMs: 8000 },
    )
    await loadHosts()
    const index = filteredHosts().findIndex((item) => item.id === hostID)
    if (index >= 0) cursor = index
    return true
  } catch (error) {
    transientText = `Host: ${String(error)}`
    return false
  } finally {
    uiDirty()
  }
}

async function toggleFavorite(): Promise<void> {
  const host = filteredHosts()[cursor]
  if (!host) return
  if (await applyHostOverride(host.id, { favorite: !host.favorite })) {
    transientText = host.favorite ? `${host.alias} ya no es favorito` : `${host.alias} es favorito`
  }
  uiDirty()
}

async function cycleHostColor(): Promise<void> {
  const host = filteredHosts()[cursor]
  if (!host) return
  const current = normalizeHex(host.color) ?? ""
  const index = HOST_COLORS.findIndex((color) => normalizeHex(color) === current)
  const next = HOST_COLORS[(index + 1) % HOST_COLORS.length]
  if (await applyHostOverride(host.id, { color: next })) {
    transientText = next ? `Color de ${host.alias}: ${next}` : `Color de ${host.alias}: automático`
  }
  uiDirty()
}

function openLabelsEditor(): void {
  const host = filteredHosts()[cursor]
  if (!host) return
  labelsEditing = true
  labelsHostID = host.id
  labelsInput = (host.labels ?? []).join(" ")
  uiDirty()
}

async function handleLabelsKey(key: KeyEvent): Promise<void> {
  const name = key.name.toLowerCase()
  if (name === "escape") {
    labelsEditing = false
  } else if (name === "return" || name === "enter") {
    await saveLabels()
    return
  } else if (name === "backspace" || name === "delete") {
    labelsInput = name === "delete" ? "" : labelsInput.slice(0, -1)
  } else if (!key.ctrl && !key.meta && key.sequence) {
    const chars = Array.from(key.sequence)
    if (chars.length === 1 && chars[0] >= " ") labelsInput += chars[0]
  }
  uiDirty()
}

function openHostMenu(host: HostProfile): void {
  hostMenuAlias = host.alias
  hostMenuCursor = 0
  hostMenuVisible = true
  actionBarFocused = false
  uiDirty()
}

function closeHostMenu(): void {
  hostMenuVisible = false
  uiDirty()
}

function openDeleteConfirm(host: HostProfile): void {
  hostMenuVisible = false
  deleteConfirmAlias = host.alias
  deleteConfirmCursor = 0
  deleteConfirmVisible = true
  uiDirty()
}

async function deleteHostAlias(alias: string): Promise<void> {
  if (!client) return
  try {
    await client.call("hosts.delete", { alias }, { timeoutMs: 8000 })
    await loadHosts()
    transientText = `Host ${alias} eliminado`
  } catch (error) {
    transientText = `No se pudo eliminar ${alias}: ${String(error)}`
  }
  deleteConfirmVisible = false
  uiDirty()
}

function handleHostMenuKey(key: KeyEvent): void {
  const name = key.name.toLowerCase()
  if (name === "escape" || name === "left") {
    closeHostMenu()
    return
  }
  if (name === "up") hostMenuCursor = Math.max(0, hostMenuCursor - 1)
  else if (name === "down") hostMenuCursor = Math.min(2, hostMenuCursor + 1)
  else if (name === "home") hostMenuCursor = 0
  else if (name === "end") hostMenuCursor = 2
  else if (name === "return" || name === "enter") {
    executeHostMenuItem(hostMenuCursor)
    return
  }
  uiDirty()
}

function handleDeleteConfirmKey(key: KeyEvent): void {
  const name = key.name.toLowerCase()
  if (name === "escape") {
    deleteConfirmVisible = false
    uiDirty()
    return
  }
  if (name === "up" || name === "left") {
    deleteConfirmCursor = Math.max(0, deleteConfirmCursor - 1)
  } else if (name === "down" || name === "right") {
    deleteConfirmCursor = Math.min(1, deleteConfirmCursor + 1)
  } else if (name === "return" || name === "enter") {
    if (deleteConfirmCursor === 0) void deleteHostAlias(deleteConfirmAlias)
    else {
      deleteConfirmVisible = false
      uiDirty()
    }
  }
  uiDirty()
}

function handleHostKey(key: KeyEvent): void {
  const name = key.name.toLowerCase()
  const all = filteredHosts()

  if (matchesAction(key, keybindings, "favorite")) void toggleFavorite()
  else if (matchesAction(key, keybindings, "color")) void cycleHostColor()
  else if (matchesAction(key, keybindings, "labels")) openLabelsEditor()
  else if (name === "left") {
    actionBarFocused = true
    actionBarCursor = 0
    uiDirty()
    return
  } else if (name === "right") {
    const host = all[cursor]
    if (host) openHostMenu(host)
    else {
      actionBarFocused = true
      actionBarCursor = 0
    }
    uiDirty()
    return
  } else if (name === "up") cursor = Math.max(0, cursor - 1)
  else if (name === "down") cursor = Math.min(Math.max(0, all.length - 1), cursor + 1)
  else if (name === "pageup") cursor = Math.max(0, cursor - 10)
  else if (name === "pagedown") cursor = Math.min(Math.max(0, all.length - 1), cursor + 10)
  else if (name === "home") cursor = 0
  else if (name === "end") cursor = Math.max(0, all.length - 1)
  else if (name === "return" || name === "enter") {
    const host = all[cursor]
    if (host) void connectHost(host)
  } else if (name === "escape") {
    if (query) {
      query = ""
      cursor = 0
    } else if (store.active()) {
      focusTerminal()
      return
    }
  } else if (name === "backspace" || name === "delete") {
    query = name === "delete" ? "" : query.slice(0, -1)
    cursor = 0
  } else if (name === "/") {
    commandMode = true
    commandInput = ""
    caretPos = 0
    query = ""
    cursor = 0
  } else if (!key.ctrl && !key.meta && key.sequence) {
    const chars = Array.from(key.sequence)
    if (chars.length === 1 && chars[0] >= " ") {
      query += chars[0]
      cursor = 0
    }
  }
  uiDirty()
}

function handleCommandKey(key: KeyEvent): void {
  const name = key.name.toLowerCase()
  const position = Math.max(0, Math.min(caretPos, commandInput.length))
  if (name === "escape") {
    commandMode = false
    commandInput = ""
  } else if (name === "left") {
    caretPos = Math.max(0, position - 1)
  } else if (name === "right") {
    caretPos = Math.min(commandInput.length, position + 1)
  } else if (name === "home") {
    caretPos = 0
  } else if (name === "end") {
    caretPos = commandInput.length
  } else if (name === "backspace") {
    if (position > 0) {
      commandInput = commandInput.slice(0, position - 1) + commandInput.slice(position)
      caretPos = position - 1
    }
  } else if (name === "delete") {
    commandInput = commandInput.slice(0, position) + commandInput.slice(position + 1)
    caretPos = position
  } else if (name === "return" || name === "enter") {
    runSlashCommand()
    return
  } else if (!key.ctrl && !key.meta && key.sequence) {
    const chars = Array.from(key.sequence)
    if (chars.length === 1 && chars[0] >= " ") {
      commandInput = commandInput.slice(0, position) + chars[0] + commandInput.slice(position)
      caretPos = position + 1
    }
  }
  uiDirty()
}

function runSlashCommand(): void {
  const raw = commandInput.trim()
  commandMode = false
  commandInput = ""
  if (!raw) {
    uiDirty()
    return
  }
  const [name, ...rest] = raw.split(/\s+/)
  const match = slashCommands.find((command) => command.name.startsWith(name.toLowerCase()))
  if (!match) {
    transientText = `Comando no encontrado: /${name}`
    uiDirty()
    return
  }
  match.run(rest.join(" "))
  uiDirty()
}

renderer.keyInput.on("keypress", (key) => {
  const name = key.name.toLowerCase()
  if (welcomeVisible) {
    key.stopPropagation()
    if (name === "return" || name === "enter" || name === "escape") {
      welcomeVisible = false
      uiDirty()
    }
    return
  }
  if (hostKeyDialog) {
    key.stopPropagation()
    handleHostKeyDialogKey(key)
    return
  }
  if (name === "f1") {
    key.stopPropagation()
    helpVisible = !helpVisible
    uiDirty()
    return
  }
  if (helpVisible) {
    key.stopPropagation()
    if (name === "escape" || name === "return" || name === "enter") helpVisible = false
    uiDirty()
    return
  }
  if (name === "f6") {
    key.stopPropagation()
    actionBarFocused = !actionBarFocused
    if (actionBarFocused) actionBarCursor = 0
    uiDirty()
    return
  }
  if (actionBarFocused) {
    key.stopPropagation()
    if (name === "left" || name === "backtab") actionBarCursor = (actionBarCursor + actionBarButtons.length - 1) % actionBarButtons.length
    else if (name === "right" || name === "tab") actionBarCursor = (actionBarCursor + 1) % actionBarButtons.length
    else if (name === "escape") actionBarFocused = false
    else if (name === "return" || name === "enter") {
      const button = actionBarButtons[actionBarCursor]
      actionBarFocused = false
      button?.run()
    }
    uiDirty()
    return
  }
  if (hostMenuVisible) {
    key.stopPropagation()
    handleHostMenuKey(key)
    return
  }
  if (deleteConfirmVisible) {
    key.stopPropagation()
    handleDeleteConfirmKey(key)
    return
  }
  if (commandMode) {
    key.stopPropagation()
    handleCommandKey(key)
    return
  }
  if (actionsMenuVisible) {
    key.stopPropagation()
    handleActionsMenuKey(key)
    return
  }
  if (newHostVisible) {
    key.stopPropagation()
    void handleNewHostKey(key)
    return
  }
  if (nmapConfirmVisible) {
    key.stopPropagation()
    if (name === "escape") {
      nmapConfirmVisible = false
    } else if (name === "return" || name === "enter") {
      void runNmapScan()
    } else if (name === "backspace" || name === "delete") {
      nmapPorts = name === "delete" ? "" : nmapPorts.slice(0, -1)
    } else if (!key.ctrl && !key.meta && key.sequence) {
      const chars = Array.from(key.sequence)
      if (chars.length === 1 && /[0-9,-]/.test(chars[0])) nmapPorts += chars[0]
    }
    uiDirty()
    return
  }
  if (transferVisible) {
    key.stopPropagation()
    void handleTransferKey(key)
    return
  }
  if (preferencesVisible) {
    key.stopPropagation()
    handlePreferencesKey(key)
    return
  }
  if (diagVisible) {
    key.stopPropagation()
    if (name === "escape" || name === "return" || name === "enter") diagVisible = false
    uiDirty()
    return
  }
  if (labelsEditing) {
    key.stopPropagation()
    void handleLabelsKey(key)
    return
  }
  if (closeDialogVisible) {
    key.stopPropagation()
    handleCloseDialogKey(key)
    return
  }
  if (frpVisible) {
    key.stopPropagation()
    void handleFRPKey(key)
    return
  }
  if (matchesAction(key, keybindings, "frp")) {
    key.stopPropagation()
    void openFRP()
    return
  }
  if (paletteVisible) {
    key.stopPropagation()
    handlePaletteKey(key)
    return
  }
  if (workspacePickerVisible) {
    key.stopPropagation()
    handleWorkspaceKey(key)
    return
  }
  if (matchesAction(key, keybindings, "palette")) {
    key.stopPropagation()
    openPalette()
    return
  }
  if (matchesAction(key, keybindings, "workspaces")) {
    key.stopPropagation()
    openWorkspacePicker()
    return
  }
  if (matchesAction(key, keybindings, "quit")) {
    key.stopPropagation()
    void quit()
    return
  }
  if (matchesAction(key, keybindings, "hosts")) {
    key.stopPropagation()
    focusHosts()
    return
  }
  if (matchesAction(key, keybindings, "tmux")) {
    key.stopPropagation()
    tmuxMode = !tmuxMode
    uiDirty()
    return
  }
  if (matchesAction(key, keybindings, "import")) {
    key.stopPropagation()
    void importHosts()
    return
  }
  if (matchesAction(key, keybindings, "close")) {
    key.stopPropagation()
    openCloseDialog()
    return
  }
  if (hostFocused && key.ctrl && name === "n") {
    key.stopPropagation()
    openNewHost()
    return
  }
  if (key.ctrl && key.shift && (name === "left" || name === "right")) {
    key.stopPropagation()
    moveActiveTab(name === "left" ? -1 : 1)
    return
  }
  if (key.ctrl && name === "tab") {
    key.stopPropagation()
    cycle(key.shift ? -1 : 1)
    return
  }
  if (key.ctrl && /^[1-9]$/.test(name)) {
    key.stopPropagation()
    const tab = store.list()[Number.parseInt(name, 10) - 1]
    if (tab) activate(tab.id)
    return
  }
  if (!hostFocused && (name === "return" || name === "enter")) {
    const active = store.active()
    if (active && !sessions.has(active.sessionId) && pendingHosts.has(active.id)) {
      key.stopPropagation()
      reconnectActive()
      return
    }
  }
  if (hostFocused && name === "tab") {
    key.stopPropagation()
    actionBarFocused = true
    actionBarCursor = 0
    uiDirty()
    return
  }
  if (hostFocused) {
    key.stopPropagation()
    handleHostKey(key)
  }
})

renderer.keyInput.on("paste", (event: { bytes: Uint8Array }) => {
  if (welcomeVisible) return
  const raw = new TextDecoder().decode(event.bytes).slice(0, 64 * 1024)
  if (!raw) return
  const oneLine = raw.replace(/\r/g, "").replace(/\n/g, " ").slice(0, 4096)

  if (commandMode) {
    commandInput += oneLine
    uiDirty()
    return
  }

  if (newHostVisible && newHostForm) {
    const form = newHostForm
    if (form.field === 2) {
      form.port += oneLine.replace(/[^0-9]/g, "")
    } else if (form.field === 0) form.alias += oneLine
    else if (form.field === 1) form.hostname += oneLine
    else if (form.field === 3) form.username += oneLine
    else if (form.field === 4) form.identityFile += oneLine
    else if (form.field === 5) form.password += oneLine
    uiDirty()
    return
  }
  if (transferVisible && transferForm) {
    if (transferForm.field === 0) transferForm.localPath += oneLine
    else if (transferForm.field === 1) transferForm.remotePath += oneLine
    uiDirty()
    return
  }
  if (frpVisible && frpForm) {
    if (frpForm.field === 0) frpForm.name += oneLine
    else if (frpForm.field === 2) frpForm.servicePort += oneLine.replace(/[^0-9]/g, "")
    else if (frpForm.field === 3) frpForm.remotePort += oneLine.replace(/[^0-9]/g, "")
    else if (frpForm.field === 4) frpForm.bindPort += oneLine.replace(/[^0-9]/g, "")
    uiDirty()
    return
  }
  if (workspacePickerVisible) {
    if (workspaceNewMode) workspaceInput += oneLine
    uiDirty()
    return
  }
  if (labelsEditing) {
    labelsInput += oneLine
    uiDirty()
    return
  }
  if (nmapConfirmVisible) {
    nmapPorts += oneLine.replace(/[^0-9,-]/g, "")
    uiDirty()
    return
  }
  if (
    helpVisible || diagVisible || preferencesVisible || paletteVisible || actionsMenuVisible ||
    closeDialogVisible || hostKeyDialog !== null || transferVisible || frpVisible || workspacePickerVisible ||
    hostMenuVisible || deleteConfirmVisible
  ) {
    return
  }
  if (!hostFocused) {
    const active = store.active()
    if (active && client && sessions.has(active.sessionId)) {
      client.sendBinary(active.sessionId, new TextEncoder().encode(raw))
      return
    }
  }
  query += oneLine
  cursor = 0
  uiDirty()
})

process.on("SIGWINCH", () => {
  const cols = process.stdout.columns
  const rows = process.stdout.rows
  if (cols > 0 && rows > 0) {
    try {
      renderer.resize(cols, rows)
    } catch (error) {
      debug(`SIGWINCH: ${String(error)}`)
    }
  }
})
renderer.on("resize", () => uiDirty())
store.subscribe(() => {
  uiDirty()
  scheduleWorkspaceSave()
})

async function loadVaultStatus(): Promise<void> {
  if (!client) return
  try {
    const status = await client.call("vault.status", {}, { timeoutMs: 5000 })
    vaultState = status.provider === "none" ? "sin vault" : status.locked ? "bloqueado" : `${status.profiles} hosts`
    vaultAgent =
      status.agentProvider && status.agentProvider !== "none"
        ? `${status.agentProvider}(${status.agentIdentities ?? 0})`
        : "sin agente"
  } catch (error) {
    debug(`vault.status: ${String(error)}`)
  }
}

async function loadHosts(): Promise<void> {
  if (!client) return
  const result = await client.call("hosts.list", {}, { timeoutMs: 5000 })
  hosts = [...(result.hosts ?? [])].sort((a, b) => {
    if (Boolean(a.favorite) !== Boolean(b.favorite)) return a.favorite ? -1 : 1
    return a.alias.localeCompare(b.alias)
  })
  conflicts = result.conflicts?.length ?? 0
}

async function diagnoseCurrentHost(): Promise<void> {
  if (!client) return
  const host = currentFormHost()
  if (!host) {
    transientText = "Diagnóstico: no hay host seleccionado"
    uiDirty()
    return
  }
  try {
    const plan = await client.call("routes.plan", { hostProfileId: host.id }, { timeoutMs: 3000 })
    const result = await client.call(
      "diagnostics.host",
      { hostProfileId: host.id, routeId: plan.selected },
      { timeoutMs: 15_000 },
    )
    const dns = result.dns.error
      ? `error: ${result.dns.error}`
      : `${result.dns.addresses.join(", ")} (${result.dns.durationMs}ms)`
    const tcp = `${result.tcp.state} (${result.tcp.latencyMs}ms)${result.tcp.error ? ` · ${result.tcp.error}` : ""}`
    const banner = result.banner.error
      ? `error: ${result.banner.error}`
      : result.banner.received || "(sin banner)"
    const tls = result.tls.applicable
      ? `${result.tls.subject ?? "certificado"} · ${result.tls.notAfter ?? ""}`
      : result.tls.message || "no aplica"
    diagContent = [
      "",
      `  Diagnóstico · ${host.alias}`,
      "",
      `  Ruta: ${result.routeKind} ${result.routeId}`,
      `  Destino: ${result.address}`,
      `  DNS: ${dns}`,
      `  TCP: ${tcp}`,
      `  Banner: ${banner}`,
      `  TLS: ${tls}`,
      "",
      "  Esc/Enter cerrar",
    ].join("\n")
    diagVisible = true
  } catch (error) {
    transientText = `Diagnóstico: ${String(error)}`
  }
  uiDirty()
}

async function exportDiagnosticsReport(): Promise<void> {
  if (!client) return
  try {
    const report = await client.call("diagnostics.report", {}, { timeoutMs: 8000 })
    const path = exportDiagnostics(report)
    transientText = `Diagnóstico: ${path}`
  } catch (error) {
    transientText = `Diagnóstico: ${String(error)}`
  }
  uiDirty()
}

async function probeCurrentHost(): Promise<void> {
  if (!client) return
  const host = currentFormHost()
  if (!host) {
    transientText = "Ruta: no hay host seleccionado"
    uiDirty()
    return
  }
  try {
    const plan = await client.call("routes.plan", { hostProfileId: host.id }, { timeoutMs: 3000 })
    const result = await client.call(
      "routes.probe",
      { hostProfileId: host.id, routeId: plan.selected },
      { timeoutMs: 8000 },
    )
    const detail = result.message ? ` · ${result.message}` : ""
    transientText = `Ruta ${result.kind} ${result.address}: ${result.state} (${result.latencyMs}ms)${detail}`
  } catch (error) {
    transientText = `Ruta: ${String(error)}`
  }
  uiDirty()
}

async function importHosts(): Promise<void> {
  if (!client) return
  try {
    const result = await client.call(
      "hosts.import",
      { source: "keepass" },
      { timeoutMs: 30_000 },
    )
    await loadHosts()
    await loadVaultStatus()
    transientText = `KeePass: ${result.imported} hosts, ${result.conflicts?.length ?? 0} conflictos`
  } catch (error) {
    transientText = `Importar KeePass: ${String(error)}`
  }
  uiDirty()
}


function renderWorkspacePicker(): void {
  if (!workspacePickerVisible) {
    workspaceOverlay.visible = false
    return
  }
  workspaceOverlay.visible = true
  const regions: OverlayRegion[] = []
  const lines = ["", workspaceNewMode ? "  Nuevo workspace" : "  Workspaces", ""]
  if (workspaceNewMode) {
    lines.push(`  Nombre: ${workspaceInput}▌`, "")
    regions.push(...addActionLine(lines, "Acciones", [
      { label: "Crear y abrir", run: () => { if (workspaceInput.trim()) void switchWorkspace(workspaceInput) } },
      { label: "Cancelar", run: () => { workspaceNewMode = false; workspaceInput = ""; uiDirty() } },
    ]))
    lines.push("", "  Escribe un nombre · Enter crear/abrir · Esc volver")
    setOverlayRegions("workspace", regions)
    workspaceOverlay.content = styledOverlay(lines, 3, [1])
    return
  }
  for (let index = 0; index < workspaceNames.length; index++) {
    const line = lines.length
    const current = workspaceNames[index] === currentWorkspace ? "  (actual)" : ""
    lines.push(`  ${index === workspacePickerCursor ? "▸" : " "} ${workspaceNames[index]}${current}`)
    regions.push({
      line,
      onHover: () => {
        workspacePickerCursor = index
      },
      onClick: () => void switchWorkspace(workspaceNames[index]),
    })
  }
  const newLine = lines.length
  lines.push(`  ${workspacePickerCursor === workspaceNames.length ? "▸" : " "} + Nuevo workspace…`)
  regions.push({
    line: newLine,
    onHover: () => {
      workspacePickerCursor = workspaceNames.length
    },
    onClick: () => {
      workspaceNewMode = true
      workspaceInput = ""
      uiDirty()
    },
  })
  lines.push("")
  regions.push(...addActionLine(lines, "Acciones", [
    { label: "Nuevo", run: () => { workspaceNewMode = true; workspaceInput = ""; uiDirty() } },
    { label: "Cerrar", run: closeWorkspacePicker },
  ]))
  lines.push("", "  ↑↓←→ mover · Enter abrir · n nuevo · Esc cerrar · clic para elegir")
  setOverlayRegions("workspace", regions)
  workspaceOverlay.content = styledOverlay(lines, 3 + workspacePickerCursor, [1])
}

function openWorkspacePicker(): void {
  workspaceNames = listWorkspaceNames()
  if (!workspaceNames.includes(currentWorkspace)) {
    workspaceNames.push(currentWorkspace)
    workspaceNames.sort((a, b) => a.localeCompare(b))
  }
  workspacePickerCursor = Math.max(0, workspaceNames.indexOf(currentWorkspace))
  workspaceNewMode = false
  workspaceInput = ""
  workspacePickerVisible = true
  uiDirty()
}

function closeWorkspacePicker(): void {
  workspacePickerVisible = false
  workspaceNewMode = false
  workspaceInput = ""
  uiDirty()
}

async function switchWorkspace(name: string): Promise<void> {
  if (!client) return
  const target = safeWorkspaceName(name)
  if (saveTimer) {
    clearTimeout(saveTimer)
    saveTimer = undefined
  }
  saveWorkspace(snapshotWorkspace(), currentWorkspace)
  restoringWorkspace = true
  store.clear()
  pendingHosts.clear()
  currentWorkspace = target
  const listed = await client
    .call("sessions.list", {}, { timeoutMs: 3000 })
    .catch(() => ({ sessions: [] as SessionInfo[] }))
  await restoreWorkspace(listed.sessions, loadWorkspace(target))
  restoringWorkspace = false
  closeWorkspacePicker()
  uiDirty()
}

function handleWorkspaceKey(key: KeyEvent): void {
  const name = key.name.toLowerCase()
  if (workspaceNewMode) {
    if (name === "escape") {
      workspaceNewMode = false
      workspaceInput = ""
    } else if (name === "backspace") {
      workspaceInput = workspaceInput.slice(0, -1)
    } else if (name === "return" || name === "enter") {
      if (workspaceInput.trim()) void switchWorkspace(workspaceInput)
    } else if (!key.ctrl && !key.meta && key.sequence) {
      const chars = Array.from(key.sequence)
      if (chars.length === 1 && chars[0] >= " ") workspaceInput += chars[0]
    }
    uiDirty()
    return
  }

  if (name === "up" || name === "left") workspacePickerCursor = Math.max(0, workspacePickerCursor - 1)
  else if (name === "down" || name === "right") {
    workspacePickerCursor = Math.min(workspaceNames.length, workspacePickerCursor + 1)
  } else if (name === "home") {
    workspacePickerCursor = 0
  } else if (name === "end") {
    workspacePickerCursor = workspaceNames.length
  } else if (name === "escape") {
    closeWorkspacePicker()
    return
  } else if (name === "n") {
    workspaceNewMode = true
    workspaceInput = ""
  } else if (name === "return" || name === "enter") {
    if (workspacePickerCursor >= workspaceNames.length) {
      workspaceNewMode = true
      workspaceInput = ""
    } else {
      void switchWorkspace(workspaceNames[workspacePickerCursor])
    }
  }
  uiDirty()
}


function renderActionBar(): void {
  for (let index = 0; index < actionBarButtons.length; index++) {
    const button = actionBarButtons[index]
    const selected = (actionBarFocused && index === actionBarCursor) || actionBarHover === index
    button.node.fg = selected ? RGBA.fromInts(15, 18, 24) : RGBA.fromInts(215, 222, 235)
    button.node.bg = selected ? RGBA.fromInts(120, 190, 255) : RGBA.fromInts(38, 44, 58)
  }
}

function renderActionsMenu(): void {
  actionsMenuBox.visible = actionsMenuVisible
  if (!actionsMenuVisible) return
  const maxHeight = Math.max(8, renderer.height - 6)
  const visibleRows = Math.max(4, Math.min(actionsMenuItems.length, maxHeight - 3))
  if (actionsMenuCursor < actionsMenuScroll) actionsMenuScroll = actionsMenuCursor
  if (actionsMenuCursor >= actionsMenuScroll + visibleRows) {
    actionsMenuScroll = actionsMenuCursor - visibleRows + 1
  }
  actionsMenuScroll = Math.max(0, Math.min(actionsMenuScroll, actionsMenuItems.length - visibleRows))
  actionsMenuBox.height = visibleRows + 3
  actionsMenuTitle.content = ` Acciones · ${actionsMenuCursor + 1}/${actionsMenuItems.length}`
  for (let index = 0; index < actionsMenuRows.length; index++) {
    const selected = index === actionsMenuCursor
    const visible = index >= actionsMenuScroll && index < actionsMenuScroll + visibleRows
    actionsMenuRows[index].visible = visible
    actionsMenuRows[index].content = `  ${selected ? "▸" : " "} ${actionsMenuItems[index].label}`
    actionsMenuRows[index].fg = selected ? RGBA.fromInts(15, 18, 24) : RGBA.fromInts(219, 226, 238)
    actionsMenuRows[index].bg = selected ? RGBA.fromInts(120, 190, 255) : RGBA.fromInts(18, 22, 30)
  }
}

function openActionsMenu(): void {
  paletteVisible = false
  actionBarFocused = false
  actionsMenuVisible = true
  actionsMenuCursor = 0
  uiDirty()
}

function closeActionsMenu(): void {
  actionsMenuVisible = false
  actionBarFocused = false
  uiDirty()
}

function executeActionsMenu(): void {
  const item = actionsMenuItems[actionsMenuCursor]
  actionsMenuVisible = false
  actionBarFocused = false
  if (item) item.run()
  uiDirty()
}

function handleActionsMenuKey(key: KeyEvent): void {
  const name = key.name.toLowerCase()
  if (name === "escape") closeActionsMenu()
  else if (name === "up" || name === "left") actionsMenuCursor = Math.max(0, actionsMenuCursor - 1)
  else if (name === "down" || name === "right") actionsMenuCursor = Math.min(actionsMenuItems.length - 1, actionsMenuCursor + 1)
  else if (name === "pageup") actionsMenuCursor = Math.max(0, actionsMenuCursor - 8)
  else if (name === "pagedown") actionsMenuCursor = Math.min(actionsMenuItems.length - 1, actionsMenuCursor + 8)
  else if (name === "home") actionsMenuCursor = 0
  else if (name === "end") actionsMenuCursor = actionsMenuItems.length - 1
  else if (name === "return" || name === "enter") executeActionsMenu()
  uiDirty()
}

async function openTransferQuick(direction: "upload" | "download"): Promise<void> {
  transferVisible = true
  await refreshTransfers()
  openTransferForm(direction)
}

function withCaret(value: string, active: boolean): string {
  if (!active || !caretOn) return value
  const position = Math.max(0, Math.min(caretPos, value.length))
  return value.slice(0, position) + "▌" + value.slice(position)
}

function renderNewHost(): void {
  if (!newHostVisible || !newHostForm) {
    newHostOverlay.visible = false
    return
  }
  newHostOverlay.visible = true
  const form = newHostForm
  const marker = (index: number) => (index === form.field ? "▸" : " ")
  const masked = form.password ? "•".repeat(Math.min(form.password.length, 24)) : ""
  const lines = [
    "",
    form.mode === "edit" ? `  Editar host · ${form.editingAlias ?? form.alias}` : "  Nuevo host",
    "",
    `  ${marker(0)} Alias:        ${withCaret(form.alias, form.field === 0)}`,
    `  ${marker(1)} HostName:    ${withCaret(form.hostname, form.field === 1)}`,
    `  ${marker(2)} Puerto:      ${withCaret(form.port, form.field === 2)}`,
    `  ${marker(3)} Usuario:     ${withCaret(form.username, form.field === 3)}`,
    `  ${marker(4)} IdentityFile: ${withCaret(form.identityFile, form.field === 4)}`,
    `  ${marker(5)} Contraseña:   ${withCaret(masked, form.field === 5)}`,
    "",
    form.error ? `  ⚠ ${form.error}` : "  La contraseña se guarda codificada (#!! encPassword) en ssh_config",
    "",
  ]
  const regions: OverlayRegion[] = []
  for (let field = 0; field < 6; field++) {
    regions.push({
      line: 3 + field,
      onHover: () => {
        form.field = field
      },
      onClick: () => {
        form.field = field
        form.error = ""
        caretPos = newHostValue(form, field).length
        uiDirty()
      },
    })
  }
  regions.push(...addActionLine(lines, "Acciones", [
    { label: form.mode === "edit" ? "Guardar y conectar" : "Crear y conectar", run: () => void saveNewHost(true) },
    { label: "Guardar sin conectar", run: () => void saveNewHost(false) },
    { label: "Cancelar", run: () => { newHostVisible = false; newHostForm = null; uiDirty() } },
  ]))
  lines.push("  ↑↓/Tab campo · ←→ cursor · Enter guardar · Ctrl+S sin conectar · Esc cancelar")
  setOverlayRegions("newhost", regions)
  newHostOverlay.content = styledOverlay(lines, 3 + form.field, [1])
}

function openNewHost(alias = ""): void {
  newHostVisible = true
  newHostForm = {
    mode: "new",
    error: "",
    field: 0,
    alias,
    hostname: "",
    port: "22",
    username: process.env.USER ?? "",
    identityFile: "",
    password: "",
  }
  uiDirty()
}

function openHostEditor(host: HostProfile): void {
  newHostVisible = true
  newHostForm = {
    mode: "edit",
    editingAlias: host.alias,
    error: "",
    field: 0,
    alias: host.alias,
    hostname: host.target.hostname,
    port: String(host.target.port || 22),
    username: host.target.username ?? "",
    identityFile: "",
    password: "",
  }
  caretPos = newHostForm.alias.length
  uiDirty()
}

function hostFormFields(_form: NonNullable<typeof newHostForm>): number[] {
  return [0, 1, 2, 3, 4, 5]
}

function newHostValue(form: NonNullable<typeof newHostForm>, field: number): string {
  switch (field) {
    case 0:
      return form.alias
    case 1:
      return form.hostname
    case 2:
      return form.port
    case 3:
      return form.username
    case 4:
      return form.identityFile
    default:
      return form.password
  }
}

function setNewHostValue(form: NonNullable<typeof newHostForm>, field: number, value: string): void {
  switch (field) {
    case 0:
      form.alias = value
      break
    case 1:
      form.hostname = value
      break
    case 2:
      form.port = value
      break
    case 3:
      form.username = value
      break
    case 4:
      form.identityFile = value
      break
    default:
      form.password = value
  }
}

async function handleNewHostKey(key: KeyEvent): Promise<void> {
  if (!newHostForm) return
  const form = newHostForm
  const name = key.name.toLowerCase()
  const fields = hostFormFields(form)
  const position = fields.indexOf(form.field)
  const moveField = (delta: number) => {
    form.field = fields[(position + delta + fields.length) % fields.length]
    form.error = ""
    caretPos = newHostValue(form, form.field).length
  }
  const value = newHostValue(form, form.field)
  if (name === "escape") {
    newHostVisible = false
    newHostForm = null
  } else if (name === "tab" || name === "down") {
    moveField(1)
  } else if (name === "up" || name === "backtab") {
    moveField(-1)
  } else if (name === "left") {
    caretPos = Math.max(0, caretPos - 1)
  } else if (name === "right") {
    caretPos = Math.min(value.length, caretPos + 1)
  } else if (name === "home") {
    caretPos = 0
  } else if (name === "end") {
    caretPos = value.length
  } else if (key.ctrl && name === "s") {
    await saveNewHost(false)
    return
  } else if (name === "return" || name === "enter") {
    await saveNewHost(form.mode === "new")
    return
  } else if (name === "backspace" || name === "delete") {
    const pos = Math.max(0, Math.min(caretPos, value.length))
    if (name === "delete") {
      setNewHostValue(form, form.field, value.slice(0, pos) + value.slice(pos + 1))
      caretPos = pos
    } else if (pos > 0) {
      setNewHostValue(form, form.field, value.slice(0, pos - 1) + value.slice(pos))
      caretPos = pos - 1
    }
  } else if (!key.ctrl && !key.meta && key.sequence) {
    const chars = Array.from(key.sequence)
    if (chars.length === 1 && chars[0] >= " ") {
      const pos = Math.max(0, Math.min(caretPos, value.length))
      if (form.field === 2 && !/[0-9]/.test(chars[0])) {
        uiDirty()
        return
      }
      setNewHostValue(form, form.field, value.slice(0, pos) + chars[0] + value.slice(pos))
      caretPos = pos + 1
    }
  }
  uiDirty()
}

async function saveNewHost(connect: boolean): Promise<void> {
  if (!client || !newHostForm) return
  const form = newHostForm
  const port = Number.parseInt(form.port, 10)
  form.alias = normalizeAlias(form.alias)
  if (!isValidAlias(form.alias)) {
    form.error = "El alias no puede contener espacios ni símbolos; usa letras, números, punto, guion o _"
    uiDirty()
    return
  }
  if (!form.hostname.trim()) {
    form.error = "HostName es obligatorio"
    uiDirty()
    return
  }
  if (/\s/.test(form.hostname.trim())) {
    form.error = "HostName no puede contener espacios"
    uiDirty()
    return
  }
  if (!Number.isFinite(port) || port < 1 || port > 65535) {
    form.error = "Puerto inválido (1–65535)"
    uiDirty()
    return
  }
  try {
    const payload = {
      alias: form.alias,
      previousAlias: form.mode === "edit" ? form.editingAlias : undefined,
      hostname: form.hostname.trim(),
      port,
      username: form.username.trim(),
      identityFile: form.identityFile.trim(),
      password: form.password,
    }
    const profile =
      form.mode === "edit"
        ? await client.call("hosts.update", payload, { timeoutMs: 8000 })
        : await client.call("hosts.create", payload, { timeoutMs: 8000 })
    await loadHosts()
    newHostVisible = false
    newHostForm = null
    transientText =
      form.mode === "edit" ? `Host ${profile.alias} actualizado` : `Host ${profile.alias} guardado`
    if (connect && form.mode === "new") await connectHost(profile)
    else uiDirty()
  } catch (error) {
    form.error = `No se pudo guardar: ${String(error)}`
    uiDirty()
  }
}

function renderHelp(): void {
  if (!helpVisible) {
    helpOverlay.visible = false
    return
  }
  helpOverlay.visible = true
  const lines = [
    "",
    "  Ayuda · atajos",
    "",
    "  Ctrl+T hosts        Ctrl+K paleta        Ctrl+P workspaces",
    "  Ctrl+N nuevo host   Ctrl+G tmux          Ctrl+I KeePass       Ctrl+W cerrar",
    "  Ctrl+Tab pestaña    Ctrl+1..9 pestaña    Ctrl+Shift+←/→ mover",
    "  F2 favorito         F3 color             F4 etiquetas",
    "  F10 salir           F1 ayuda             F6 barra de acciones",
    "",
    "  Ratón: clic en hosts, pestañas, botones y filas de menú",
    "  Rueda del ratón: desplaza la lista de hosts",
    "  En hosts: ↑↓ mover · ← menú superior · → acciones del host",
    "  Acciones del host (→): Conectar · Editar · Eliminar",
    "  Enter conecta · doble clic conecta · clic derecho abre acciones",
    "  Escribir busca · / comandos (/connect, /conn, /help)",
    "  El alta/edición admite contraseña; se guarda como #!! encPassword",
    "  Pegar (Ctrl+Shift+V) en búsqueda, formularios y terminal según el foco",
    "  En menús y formularios: ↑↓←→ mover · Enter aceptar · Esc volver",
    "  En FRP (Ctrl+F): ↑↓←→ proxy · n crear · d parar · x eliminar · s frps · r rotar",
    "  En transferencias: ↑↓←→ mover · Espacio cambia · u subir · d descargar · c cancelar",
    "  En Nmap: Enter confirma el escaneo · Esc cancela",
    "  La bienvenida se cierra con Enter; SSH05_NO_WELCOME=1 la omite",
    "",
  ]
  const regions = addActionLine(lines, "Acciones", [
    { label: "Cerrar", run: () => { helpVisible = false; uiDirty() } },
  ])
  lines.push("", "  Esc/Enter cerrar · clic en [ Cerrar ] o en el título")
  regions.push({
    line: 1,
    onClick: () => {
      helpVisible = false
      uiDirty()
    },
  })
  setOverlayRegions("help", regions)
  helpOverlay.content = styledOverlay(lines, -1, [1])
}

function renderNmapConfirm(): void {
  if (!nmapConfirmVisible) {
    nmapOverlay.visible = false
    return
  }
  nmapOverlay.visible = true
  const host = currentFormHost()
  const lines = [
    "",
    "  Escaneo Nmap optativo",
    "",
    `  Host: ${host?.alias ?? "(ninguno)"}`,
    `  Puertos: ${nmapPorts}▌`,
    "",
    "  Solo úsalo en redes autorizadas. Se ejecutará nmap -sT -Pn.",
    "",
  ]
  const regions = addActionLine(lines, "Acciones", [
    { label: "Escanear", run: () => void runNmapScan() },
    { label: "Cancelar", run: () => { nmapConfirmVisible = false; uiDirty() } },
  ])
  lines.push("", "  Enter escanear · Esc cancelar · edita los puertos con el teclado · clic para elegir")
  setOverlayRegions("nmap", regions)
  nmapOverlay.content = styledOverlay(lines, 4, [1])
}

function openNmapConfirm(): void {
  const host = currentFormHost()
  if (!host) {
    transientText = "Nmap: no hay host seleccionado"
    uiDirty()
    return
  }
  nmapConfirmVisible = true
  uiDirty()
}

async function runNmapScan(): Promise<void> {
  if (!client) return
  const host = currentFormHost()
  if (!host) return
  nmapConfirmVisible = false
  try {
    transientText = `Nmap: escaneando ${host.alias}…`
    uiDirty()
    const plan = await client.call("routes.plan", { hostProfileId: host.id }, { timeoutMs: 3000 })
    const result = await client.call(
      "diagnostics.nmap",
      {
        hostProfileId: host.id,
        routeId: plan.selected,
        ports: nmapPorts,
        confirm: true,
        authorized: true,
      },
      { timeoutMs: 70_000 },
    )
    const lines = ["", `  Nmap · ${host.alias}`, "", `  Destino: ${result.target}`, ""]
    if (result.ports.length === 0) lines.push("  Sin puertos devueltos")
    for (const port of result.ports) {
      const service = [port.service, port.product, port.version].filter(Boolean).join(" ")
      lines.push(`  ${port.port}/${port.protocol} ${port.state} ${service}`)
    }
    lines.push("", "  Esc/Enter cerrar")
    diagContent = lines.join("\n")
    diagVisible = true
    transientText = ""
  } catch (error) {
    transientText = `Nmap: ${String(error)}`
  }
  uiDirty()
}

async function showNetworkInfo(): Promise<void> {
  if (!client) return
  const host = currentFormHost()
  if (!host) {
    transientText = "Red: no hay host seleccionado"
    uiDirty()
    return
  }
  try {
    transientText = `Red: consultando ${host.alias}…`
    uiDirty()
    const plan = await client.call("routes.plan", { hostProfileId: host.id }, { timeoutMs: 3000 })
    const result = await client.call(
      "diagnostics.network",
      { hostProfileId: host.id, routeId: plan.selected },
      { timeoutMs: 15_000 },
    )
    const asn = result.asn.error
      ? `error: ${result.asn.error}`
      : `${result.asn.asn ?? "?"} · ${result.asn.prefix ?? "?"} · ${result.asn.country ?? "?"} · ${result.asn.registry ?? "?"}`
    const rdap = result.rdap.error
      ? `error: ${result.rdap.error}`
      : `${result.rdap.name ?? "?"} (${result.rdap.handle ?? "?"}) · ${result.rdap.startAddress ?? "?"}–${result.rdap.endAddress ?? "?"}`
    diagContent = [
      "",
      `  Red · ${host.alias}`,
      "",
      `  Host: ${result.host}`,
      `  Direcciones: ${result.addresses.join(", ") || "sin resolver"}`,
      `  ASN: ${asn}`,
      `  RDAP: ${rdap}`,
      `  País orientativo: ${result.geoCountry ?? "desconocido"}`,
      `  Precisión: ${result.geoPrecision}`,
      "",
      "  Procedencia:",
      ...result.provenance.map((item) => `   · ${item}`),
      "",
      "  Esc/Enter cerrar",
    ].join("\n")
    diagVisible = true
    transientText = ""
  } catch (error) {
    transientText = `Red: ${String(error)}`
  }
  uiDirty()
}

function renderTransfers(): void {
  if (!transferVisible) {
    transferOverlay.visible = false
    return
  }
  transferOverlay.visible = true
  if (transferForm) {
    const form = transferForm
    const rows = [
      ["Local", form.localPath],
      ["Remoto", form.remotePath],
      ["Protocolo", form.protocol],
      ["Recursivo", form.recursive ? "sí" : "no"],
    ]
    const lines = [
      "",
      `  ${form.direction === "upload" ? "Subir" : "Descargar"} archivo`,
      "",
      ...rows.map(([label, value], index) =>
        `  ${form.field === index ? "▸" : " "} ${label}: ${value}${form.field === index && caretOn ? "▌" : ""}`,
      ),
      "",
    ]
    const regions: OverlayRegion[] = rows.map((_, index) => ({
      line: 3 + index,
      onHover: () => {
        form.field = index
      },
      onClick: () => {
        form.field = index
        uiDirty()
      },
    }))
    regions.push(...addActionLine(lines, "Acciones", [
      { label: "Iniciar", run: () => void startTransferForm() },
      { label: "Cancelar", run: () => { transferForm = null; uiDirty() } },
    ]))
    lines.push("  ↑↓←→ campo · Espacio cambia · Enter iniciar · Esc cancelar · clic para elegir")
    lines.push("  Ctrl+P protocolo · Ctrl+R recursivo")
    setOverlayRegions("transfer", regions)
    transferOverlay.content = styledOverlay(lines, 3 + form.field, [1])
    return
  }
  const lines = ["", "  Transferencias", ""]
  const regions: OverlayRegion[] = []
  const count = Math.min(transfers.length, 8)
  if (count === 0) lines.push("  Sin transferencias")
  for (let index = 0; index < count; index++) {
    const item = transfers[index]
    const arrow = item.direction === "upload" ? "→" : "←"
    const error = item.errorLabel ? ` · ${item.errorLabel}` : ""
    const line = lines.length
    lines.push(
      `  ${index === transferCursor ? "▸" : " "} ${item.state} ${item.protocol} ${item.localPath} ${arrow} ${item.remotePath}${error}`,
    )
    regions.push({
      line,
      onHover: () => {
        transferCursor = index
      },
      onClick: () => {
        transferCursor = index
        uiDirty()
      },
    })
  }
  lines.push("")
  regions.push(...addActionLine(lines, "Acciones", [
    { label: "Subir", run: () => openTransferForm("upload") },
    { label: "Descargar", run: () => openTransferForm("download") },
    { label: "Cancelar", run: () => void cancelSelectedTransfer() },
    { label: "Cerrar", run: closeTransfers },
  ]))
  lines.push("", "  ↑↓←→ mover · u subir · d descargar · c cancelar · Esc cerrar · clic para elegir")
  setOverlayRegions("transfer", regions)
  transferOverlay.content = styledOverlay(lines, count > 0 ? 3 + transferCursor : -1, [1])
}

async function cancelSelectedTransfer(): Promise<void> {
  const item = transfers[transferCursor]
  if (!item || !client) return
  try {
    await client.call("transfers.cancel", { transferId: item.transferId }, { timeoutMs: 5000 })
    await refreshTransfers()
  } catch (error) {
    transientText = `Cancelar transferencia: ${String(error)}`
    uiDirty()
  }
}

async function refreshTransfers(): Promise<void> {
  if (!client) return
  try {
    const result = await client.call("transfers.list", {}, { timeoutMs: 5000 })
    transfers = result.transfers ?? []
    transferCursor = Math.min(transferCursor, Math.max(0, transfers.length - 1))
  } catch (error) {
    debug(`transfers.list: ${String(error)}`)
  }
  uiDirty()
}

async function openTransfers(): Promise<void> {
  transferVisible = true
  transferForm = null
  transferCursor = 0
  await refreshTransfers()
  stopTransferPoll()
  transferPoll = setInterval(() => void refreshTransfers(), 700)
}

function closeTransfers(): void {
  transferVisible = false
  transferForm = null
  stopTransferPoll()
  uiDirty()
}

function stopTransferPoll(): void {
  if (transferPoll) {
    clearInterval(transferPoll)
    transferPoll = undefined
  }
}

function openTransferForm(direction: "upload" | "download"): void {
  const host = currentFormHost()
  if (!host) {
    transientText = "Transferencia: no hay host seleccionado"
    uiDirty()
    return
  }
  transferForm = {
    direction,
    protocol: "sftp",
    localPath: direction === "upload" ? `${process.cwd()}/` : `${process.env.HOME ?? "~"}/`,
    remotePath: "~/",
    recursive: false,
    field: 0,
  }
  uiDirty()
}

async function startTransferForm(): Promise<void> {
  if (!client || !transferForm) return
  const form = transferForm
  const host = currentFormHost()
  if (!host) {
    transientText = "Transferencia: no hay host seleccionado"
    return
  }
  try {
    const plan = await client.call("routes.plan", { hostProfileId: host.id }, { timeoutMs: 3000 })
    await client.call(
      "transfers.start",
      {
        hostProfileId: host.id,
        routeId: plan.selected,
        protocol: form.protocol,
        direction: form.direction,
        localPath: form.localPath,
        remotePath: form.remotePath,
        recursive: form.recursive,
      },
      { timeoutMs: 15_000 },
    )
    transferForm = null
    await refreshTransfers()
  } catch (error) {
    transientText = `Transferencia: ${String(error)}`
    uiDirty()
  }
}

async function handleTransferKey(key: KeyEvent): Promise<void> {
  const name = key.name.toLowerCase()
  const space = name === "space" || key.sequence === " "
  if (transferForm) {
    const form = transferForm
    if (name === "escape") {
      transferForm = null
    } else if (name === "tab" || name === "down" || name === "right") {
      form.field = (form.field + 1) % 4
    } else if (name === "up" || name === "left" || name === "backtab") {
      form.field = (form.field + 3) % 4
    } else if (space && form.field === 2) {
      form.protocol = form.protocol === "sftp" ? "scp" : "sftp"
    } else if (space && form.field === 3) {
      form.recursive = !form.recursive
    } else if (key.ctrl && name === "p") {
      form.protocol = form.protocol === "sftp" ? "scp" : "sftp"
    } else if (key.ctrl && name === "r") {
      form.recursive = !form.recursive
    } else if (name === "backspace" || name === "delete") {
      if (form.field === 0) {
        form.localPath = name === "delete" ? "" : form.localPath.slice(0, -1)
      } else if (form.field === 1) {
        form.remotePath = name === "delete" ? "" : form.remotePath.slice(0, -1)
      }
    } else if (name === "return" || name === "enter") {
      if (form.field === 2) form.protocol = form.protocol === "sftp" ? "scp" : "sftp"
      else if (form.field === 3) form.recursive = !form.recursive
      else await startTransferForm()
      return
    } else if (!key.ctrl && !key.meta && key.sequence) {
      const chars = Array.from(key.sequence)
      if (chars.length === 1 && chars[0] >= " ") {
        if (form.field === 0) form.localPath += chars[0]
        else if (form.field === 1) form.remotePath += chars[0]
      }
    }
    uiDirty()
    return
  }

  if (name === "escape") {
    closeTransfers()
  } else if (name === "u") {
    openTransferForm("upload")
  } else if (name === "d") {
    openTransferForm("download")
  } else if (name === "up" || name === "left") {
    transferCursor = Math.max(0, transferCursor - 1)
  } else if (name === "down" || name === "right") {
    transferCursor = Math.min(Math.max(0, transfers.length - 1), transferCursor + 1)
  } else if (name === "home") {
    transferCursor = 0
  } else if (name === "end") {
    transferCursor = Math.max(0, transfers.length - 1)
  } else if (name === "pageup") {
    transferCursor = Math.max(0, transferCursor - 5)
  } else if (name === "pagedown") {
    transferCursor = Math.min(Math.max(0, transfers.length - 1), transferCursor + 5)
  } else if (name === "c") {
    await cancelSelectedTransfer()
  }
  uiDirty()
}

function openPreferences(): void {
  preferencesCursor = 0
  preferencesVisible = true
  uiDirty()
}

function renderPreferences(): void {
  if (!preferencesVisible) {
    prefsOverlay.visible = false
    return
  }
  prefsOverlay.visible = true
  const marker = (index: number) => (index === preferencesCursor ? "▸" : " ")
  const lines = [
    "",
    "  Preferencias",
    "",
    `  ${marker(0)} Ancho del panel:      ◀ ${preferences.hostPanelWidth} ▶`,
    `  ${marker(1)} Densidad:            ◀ ${preferences.density} ▶`,
    `  ${marker(2)} Posición de pestañas: ◀ ${preferences.tabPosition} ▶`,
    "",
    "  ↑↓←→ mover y ajustar · clic izquierda/derecha ajusta · w/d/t · Enter/Esc cerrar",
  ]
  setOverlayRegions(
    "prefs",
    [0, 1, 2].map((index) => ({
      line: 3 + index,
      onHover: () => {
        preferencesCursor = index
      },
      onClick: (event) => {
        preferencesCursor = index
        const delta = event.x - prefsOverlay.x < renderer.width / 2 ? -1 : 1
        adjustPreferences(index, delta)
      },
    })),
  )
  prefsOverlay.content = styledOverlay(lines, 3 + preferencesCursor, [1])
}

function applyTabPosition(): void {
  root.remove(tabBar)
  root.add(tabBar, preferences.tabPosition === "bottom" ? 1 : 0)
  renderer.requestRender()
}

function persistPreferences(): void {
  try {
    savePreferences(preferences)
  } catch (error) {
    transientText = `Preferencias: ${String(error)}`
  }
}

function resizeTerminals(): void {
  const cols = Math.max(20, renderer.width - hostPanelWidth - 2)
  const rows = Math.max(5, renderer.height - 4)
  for (const view of sessions.values()) {
    scheduleResize(view.sessionId, cols, view.terminal.height || rows)
  }
}

function adjustPreferences(cursorIndex: number, delta: number): void {
  if (cursorIndex === 0) {
    preferences.hostPanelWidth = nextPanelWidth(preferences.hostPanelWidth, delta)
    hostPanelWidth = preferences.hostPanelWidth
    hostPanel.width = hostPanelWidth
    persistPreferences()
    resizeTerminals()
    return
  }
  if (cursorIndex === 1) {
    preferences.density = preferences.density === "compact" ? "comfortable" : "compact"
    persistPreferences()
    return
  }
  preferences.tabPosition = preferences.tabPosition === "top" ? "bottom" : "top"
  applyTabPosition()
  persistPreferences()
}

function handlePreferencesKey(key: KeyEvent): void {
  const name = key.name.toLowerCase()
  if (name === "escape" || name === "return" || name === "enter") {
    preferencesVisible = false
  } else if (name === "up") {
    preferencesCursor = (preferencesCursor + 2) % 3
  } else if (name === "down") {
    preferencesCursor = (preferencesCursor + 1) % 3
  } else if (name === "home") {
    preferencesCursor = 0
  } else if (name === "end") {
    preferencesCursor = 2
  } else if (name === "left") {
    adjustPreferences(preferencesCursor, -1)
  } else if (name === "right") {
    adjustPreferences(preferencesCursor, 1)
  } else if (name === "w") {
    adjustPreferences(0, 1)
  } else if (name === "d") {
    adjustPreferences(1, 1)
  } else if (name === "t") {
    adjustPreferences(2, 1)
  }
  uiDirty()
}

function renderDiagnostics(): void {
  if (!diagVisible) {
    diagOverlay.visible = false
    return
  }
  diagOverlay.visible = true
  const lines = diagContent.split("\n")
  const titleLine = lines.findIndex((line) => line.trim().startsWith("Diagnóstico") || line.trim().startsWith("Red ·"))
  lines.push("")
  const regions = addActionLine(lines, "Acciones", [
    { label: "Cerrar", run: () => { diagVisible = false; uiDirty() } },
  ])
  lines.push("", "  Esc/Enter cerrar · clic en [ Cerrar ]")
  setOverlayRegions("diag", regions)
  diagOverlay.content = styledOverlay(lines, -1, titleLine >= 0 ? [titleLine] : [1])
}

function renderLabelsEditor(): void {
  if (!labelsEditing) {
    labelsOverlay.visible = false
    return
  }
  labelsOverlay.visible = true
  const lines = ["", "  Etiquetas del host", "", `  ${labelsInput}▌`, ""]
  const regions = addActionLine(lines, "Acciones", [
    { label: "Guardar", run: () => void saveLabels() },
    { label: "Cancelar", run: () => { labelsEditing = false; uiDirty() } },
  ])
  lines.push("", "  Enter guardar · Esc cancelar · clic para elegir")
  setOverlayRegions("labels", regions)
  labelsOverlay.content = styledOverlay(lines, 3, [1])
}

async function saveLabels(): Promise<void> {
  if (!client) return
  const labels = labelsInput.split(/\s+/).filter(Boolean)
  const hostID = labelsHostID
  labelsEditing = false
  await applyHostOverride(hostID, { labels })
  uiDirty()
}

function renderFRP(): void {
  if (!frpVisible) {
    frpOverlay.visible = false
    return
  }
  frpOverlay.visible = true
  if (frpForm) {
    const form = frpForm
    const fields = [
      `Nombre: ${form.name}`,
      `Tipo: ${form.type}  (Espacio/t)`,
      `Service port: ${form.servicePort}`,
      `Remote port: ${form.remotePort}`,
      `Bind port (visitor STCP/XTCP): ${form.bindPort}`,
      `Exposición: ${form.type === "tcp" || form.type === "udp" ? "PÚBLICA en frps" : "privada (visitor)"}`,
    ]
    const lines = ["", "  Nuevo proxy FRP", ""]
    const regions: OverlayRegion[] = []
    fields.forEach((line, index) => {
      const row = lines.length
      lines.push(`  ${index === form.field ? "▸" : " "} ${line}${index === form.field && caretOn ? "▌" : ""}`)
      regions.push({
        line: row,
        onHover: () => {
          form.field = index
        },
        onClick: () => {
          form.field = index
          uiDirty()
        },
      })
    })
    lines.push("")
    regions.push(...addActionLine(lines, "Acciones", [
      { label: "Aplicar", run: () => void applyFrpForm() },
      { label: "Cancelar", run: () => { frpForm = null; uiDirty() } },
    ]))
    lines.push("", "  ↑↓←→ campo · Espacio cambia tipo · Enter aplicar · Esc cancelar · clic para elegir")
    setOverlayRegions("frp", regions)
    frpOverlay.content = styledOverlay(lines, 3 + form.field, [1])
    return
  }
  const lines = ["", "  FRP · endpoints", ""]
  const regions: OverlayRegion[] = []
  if (frpEndpoints.length === 0) lines.push("  Sin endpoints configurados")
  for (const endpoint of frpEndpoints) {
    const kind = endpoint.managed ? "local" : "remoto"
    const state = endpoint.state ?? "stopped"
    const stats = endpoint.pid
      ? ` pid=${endpoint.pid} up=${endpoint.uptimeSeconds ?? 0}s`
      : ""
    lines.push(`   ${state} ${kind} ${endpoint.address}:${endpoint.port}${stats}`)
  }
  lines.push("", "  FRP · proxies", "")
  let proxyStart = lines.length
  if (frpProxies.length === 0) lines.push("  Sin proxies configurados")
  for (let index = 0; index < frpProxies.length; index++) {
    const proxy = frpProxies[index]
    const target =
      proxy.type === "stcp" || proxy.type === "xtcp"
        ? proxy.bindPort
          ? `visitor 127.0.0.1:${proxy.bindPort}`
          : `stcp ${proxy.servicePort}`
        : `${proxy.servicePort} → ${proxy.remotePort}`
    const stats = proxy.pid ? ` pid=${proxy.pid} up=${proxy.uptimeSeconds ?? 0}s` : ""
    const error = proxy.lastErrorLabel ? ` · ${proxy.lastErrorLabel}` : ""
    const line = lines.length
    lines.push(`  ${index === frpCursor ? "▸" : " "} ${proxy.state} ${proxy.name} [${proxy.type}] ${target}${stats}${error}`)
    regions.push({
      line,
      onHover: () => {
        frpCursor = index
        frpDeleteArmed = ""
      },
      onClick: () => {
        frpCursor = index
        frpDeleteArmed = ""
        uiDirty()
      },
    })
  }
  lines.push("")
  regions.push(...addActionLine(lines, "Acciones", [
    { label: "Nuevo", run: openFRPForm },
    { label: "Parar", run: () => void stopSelectedFRP() },
    { label: "Eliminar", run: () => void removeSelectedFRP() },
    { label: "frps", run: () => void toggleLocalFrps() },
    { label: "Rotar token", run: () => void rotateFRPToken() },
    { label: "Cerrar", run: closeFRP },
  ]))
  lines.push("", "  ↑↓←→ mover · n nuevo · d parar · x eliminar · s frps · r rotar · Esc cerrar · clic para elegir")
  setOverlayRegions("frp", regions)
  frpOverlay.content = styledOverlay(lines, frpProxies.length > 0 ? proxyStart + frpCursor : -1, [1])
}

async function refreshFRP(): Promise<void> {
  if (!client) return
  try {
    const result = await client.call("frp.instances.list", {}, { timeoutMs: 5000 })
    frpProxies = result.proxies ?? []
    frpEndpoints = result.endpoints ?? []
    frpCursor = Math.min(frpCursor, Math.max(0, frpProxies.length - 1))
  } catch (error) {
    transientText = `FRP: ${String(error)}`
  }
  uiDirty()
}

async function openFRP(): Promise<void> {
  frpVisible = true
  frpForm = null
  frpDeleteArmed = ""
  await refreshFRP()
}

function closeFRP(): void {
  frpVisible = false
  frpForm = null
  uiDirty()
}

function openFRPForm(): void {
  const host = currentFormHost()
  frpForm = {
    field: 0,
    name: host ? `proxy-${host.alias}` : "proxy",
    type: "tcp",
    servicePort: host ? String(host.target.port) : "22",
    remotePort: "",
    bindPort: "",
  }
  uiDirty()
}

async function stopSelectedFRP(): Promise<void> {
  const proxy = frpProxies[frpCursor]
  if (!proxy || !client) return
  try {
    await client.call("frp.proxies.stop", { proxyId: proxy.id }, { timeoutMs: 5000 })
    await refreshFRP()
  } catch (error) {
    transientText = `FRP stop: ${String(error)}`
    uiDirty()
  }
}

async function removeSelectedFRP(): Promise<void> {
  const proxy = frpProxies[frpCursor]
  if (!proxy || !client) return
  if (frpDeleteArmed !== proxy.id) {
    frpDeleteArmed = proxy.id
    transientText = `FRP: pulsa Eliminar otra vez para borrar ${proxy.name}`
    uiDirty()
    return
  }
  try {
    await client.call("frp.proxies.remove", { proxyId: proxy.id }, { timeoutMs: 8000 })
    frpDeleteArmed = ""
    await refreshFRP()
    transientText = `FRP: ${proxy.name} eliminado`
  } catch (error) {
    transientText = `FRP remove: ${String(error)}`
  }
  uiDirty()
}

async function toggleLocalFrps(): Promise<void> {
  if (!client) return
  const endpoint = frpEndpoints.find((item) => item.managed)
  if (!endpoint) {
    transientText = "FRP: no hay frps local gestionado"
    uiDirty()
    return
  }
  try {
    if (endpoint.state === "online") {
      await client.call("frp.servers.stop", { endpointId: endpoint.id }, { timeoutMs: 10_000 })
      transientText = "FRP: frps detenido"
    } else {
      await client.call("frp.servers.start", { endpointId: endpoint.id }, { timeoutMs: 15_000 })
      transientText = "FRP: frps iniciado"
    }
    await refreshFRP()
  } catch (error) {
    transientText = `FRP frps: ${String(error)}`
    uiDirty()
  }
}

async function rotateFRPToken(): Promise<void> {
  const endpoint = frpEndpoints[0]
  if (!endpoint || !client) return
  try {
    await client.call("frp.tokens.rotate", { endpointId: endpoint.id }, { timeoutMs: 15_000 })
    await refreshFRP()
    transientText = "FRP: token rotado"
  } catch (error) {
    transientText = `FRP rotate: ${String(error)}`
    uiDirty()
  }
}

function currentFormHost(): HostProfile | undefined {
  const active = store.active()
  if (active) {
    const view = sessions.get(active.sessionId)
    if (view) return view.host
  }
  return filteredHosts()[cursor]
}

async function applyFrpForm(): Promise<void> {
  if (!client || !frpForm) return
  const form = frpForm
  const host = currentFormHost()
  const proxy: FrpProxy = {
    id: "",
    name: form.name || `proxy-${form.servicePort || "0"}`,
    type: form.type as FrpProxy["type"],
    endpointId: frpEndpoints[0]?.id ?? "",
    hostProfileId: host?.id,
    servicePort: Number.parseInt(form.servicePort, 10) || 0,
    remotePort: Number.parseInt(form.remotePort, 10) || 0,
    bindPort: Number.parseInt(form.bindPort, 10) || 0,
    exposure: form.type === "tcp" || form.type === "udp" ? "public" : "private",
    state: "stopped",
  }
  try {
    await client.call("frp.proxies.apply", { proxy }, { timeoutMs: 30_000 })
    frpForm = null
    await refreshFRP()
    transientText = `FRP: ${proxy.name} aplicado`
  } catch (error) {
    transientText = `FRP apply: ${String(error)}`
    uiDirty()
  }
}

async function handleFRPKey(key: KeyEvent): Promise<void> {
  const name = key.name.toLowerCase()
  const space = name === "space" || key.sequence === " "
  if (frpForm) {
    const form = frpForm
    if (name === "escape") {
      frpForm = null
    } else if (name === "tab" || name === "down" || name === "right") {
      form.field = (form.field + 1) % 5
    } else if (name === "up" || name === "left" || name === "backtab") {
      form.field = (form.field + 4) % 5
    } else if (name === "t" || (space && form.field === 1)) {
      form.type = nextFRPFormType(form.type)
    } else if (name === "backspace") {
      if (form.field === 0) form.name = form.name.slice(0, -1)
      else if (form.field === 2) form.servicePort = form.servicePort.slice(0, -1)
      else if (form.field === 3) form.remotePort = form.remotePort.slice(0, -1)
      else if (form.field === 4) form.bindPort = form.bindPort.slice(0, -1)
    } else if (name === "return" || name === "enter") {
      if (form.field === 1) {
        form.type = nextFRPFormType(form.type)
      } else {
        await applyFrpForm()
        return
      }
    } else if (!key.ctrl && !key.meta && key.sequence) {
      const chars = Array.from(key.sequence)
      if (chars.length === 1 && chars[0] >= " ") {
        if (form.field === 0) form.name += chars[0]
        else if (form.field === 2) form.servicePort += chars[0]
        else if (form.field === 3) form.remotePort += chars[0]
        else if (form.field === 4) form.bindPort += chars[0]
      }
    }
    uiDirty()
    return
  }

  if (name === "escape") {
    closeFRP()
  } else if (name === "up" || name === "left") {
    frpCursor = Math.max(0, frpCursor - 1)
    frpDeleteArmed = ""
  } else if (name === "down" || name === "right") {
    frpCursor = Math.min(Math.max(0, frpProxies.length - 1), frpCursor + 1)
    frpDeleteArmed = ""
  } else if (name === "home") {
    frpCursor = 0
    frpDeleteArmed = ""
  } else if (name === "end") {
    frpCursor = Math.max(0, frpProxies.length - 1)
    frpDeleteArmed = ""
  } else if (name === "pageup") {
    frpCursor = Math.max(0, frpCursor - 5)
    frpDeleteArmed = ""
  } else if (name === "pagedown") {
    frpCursor = Math.min(Math.max(0, frpProxies.length - 1), frpCursor + 5)
    frpDeleteArmed = ""
  } else if (name === "n") {
    openFRPForm()
  } else if (name === "x" && frpProxies[frpCursor] && client) {
    await removeSelectedFRP()
  } else if (name === "d" && frpProxies[frpCursor] && client) {
    await stopSelectedFRP()
  } else if (name === "s" && client) {
    await toggleLocalFrps()
  } else if (name === "r" && frpEndpoints[0] && client) {
    await rotateFRPToken()
  }
  uiDirty()
}

function nextFRPFormType(value: string): string {
  switch (value) {
    case "tcp":
      return "udp"
    case "udp":
      return "stcp"
    case "stcp":
      return "xtcp"
    default:
      return "tcp"
  }
}

/**
 * Starts ssh05d when the control socket is missing. The packaged unit is
 * enabled globally, but this covers the first run after installation and
 * launches before the user session reloads systemd.
 */
async function ensureSupervisor(): Promise<void> {
  if (existsSync(socketPath)) return
  const candidates = [
    envValue("SUPERVISOR_BIN"),
    SUPERVISOR,
    join(import.meta.dir, "../../../bin", SUPERVISOR),
  ].filter((value): value is string => Boolean(value))
  for (const candidate of candidates) {
    try {
      const child = Bun.spawn({
        cmd: [candidate, "--socket", socketPath],
        stdin: "ignore",
        stdout: "ignore",
        stderr: "ignore",
        env: process.env,
      })
      child.unref()
      debug(`supervisor: started ${candidate}`)
    } catch (error) {
      debug(`supervisor: ${candidate} failed: ${String(error)}`)
      continue
    }
    for (let attempt = 0; attempt < 50; attempt++) {
      if (existsSync(socketPath)) return
      await Bun.sleep(100)
    }
    debug(`supervisor: ${candidate} did not create ${socketPath}`)
    return
  }
}

async function start(): Promise<void> {
  uiDirty()
  await ensureSupervisor()
  try {
    client = await IpcClient.connect(socketPath)
  } catch (error) {
    errorText = `No se pudo conectar a ${SUPERVISOR} en ${socketPath}: ${String(error)}. Comprueba el servicio ssh05d`
    uiDirty()
    return
  }

  client.setBinaryHandler((sessionId, data) => {
    const view = sessions.get(sessionId)
    if (view) {
      view.terminal.write(data)
      return
    }
    const chunks = pendingData.get(sessionId) ?? []
    let total = chunks.reduce((sum, chunk) => sum + chunk.length, 0)
    chunks.push(data)
    total += data.length
    while (total > MAX_PENDING_BYTES && chunks.length > 1) {
      total -= chunks.shift()!.length
    }
    pendingData.set(sessionId, chunks)
  })

  client.setEventHandler((event: IpcEvent) => {
    if (event.event !== "session.status") return
    const status = event.payload as SessionStatus
    if (!status || typeof status.phase !== "string") return
    const sessionId = status.sessionId ?? (event.sessionId as SessionId | undefined)
    if (!sessionId) return
    const view = sessions.get(sessionId)
    if (view) view.phase = status.phase
    const tab = store.list().find((item) => item.sessionId === sessionId)
    if (tab) {
      store.update(tab.id, { phase: status.phase })
      if (status.tmuxSession === "") store.update(tab.id, { tmux: undefined })
    }
    else pendingStatus.set(sessionId, status)
    uiDirty()
  })

  try {
    await loadHosts()
    await loadVaultStatus()
    const listed = await client
      .call("sessions.list", {}, { timeoutMs: 3000 })
      .catch(() => ({ sessions: [] as SessionInfo[] }))
    await restoreWorkspace(listed.sessions, loadWorkspace(currentWorkspace))
    uiDirty()
  } catch (error) {
    errorText = `hosts.list: ${String(error)}`
    uiDirty()
  }
}

void start()
