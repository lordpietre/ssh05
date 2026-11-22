/**
 * TRSS UI: OpenTUI + IpcClient + TabStore against the local supervisor.
 *
 * It never opens SSH directly. All sessions, input, resize and status go
 * through trssd over the typed IPC contract.
 *
 * Run:
 *   TRSS_SOCKET=/run/user/1000/trss/control.sock bun run src/trss.ts
 */

import {
  BoxRenderable,
  EmbeddedTerminalRenderable,
  type KeyEvent,
  RGBA,
  TextRenderable,
  createCliRenderer,
} from "@opentui/core"
import { tmpdir } from "node:os"
import { appendFileSync } from "node:fs"
import { join } from "node:path"
import type {
  FrpEndpoint,
  FrpProxy,
  HostProfile,
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
import { exportDiagnostics } from "./diagnostics/export.ts"
import {
  loadPreferences,
  nextPanelWidth,
  savePreferences,
  type Preferences,
} from "./preferences/store.ts"
import { normalizeHex } from "./theme/color.ts"
import { IpcClient } from "./ipc/client.ts"
import { InMemoryTabStore } from "./tabs/store.ts"
import { phaseIcon, phaseLabel, renderTabBar, renderTabBarStyled } from "./tabs/tabbar.ts"
import { listWorkspaceNames, loadWorkspace, safeWorkspaceName, saveWorkspace } from "./workspaces/store.ts"

let preferences: Preferences = loadPreferences()
let hostPanelWidth = preferences.hostPanelWidth
const MAX_SESSIONS = 16
const MAX_PENDING_BYTES = 1 << 20

const debugFile = process.env.TRSS_UI_DEBUG
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

function defaultSocketPath(): string {
  if (process.env.TRSS_SOCKET) return process.env.TRSS_SOCKET
  const runtimeDir = process.env.XDG_RUNTIME_DIR
  if (runtimeDir) return join(runtimeDir, "trss", "control.sock")
  const uid = typeof process.getuid === "function" ? process.getuid() : "user"
  return join(tmpdir(), `trss-${uid}`, "control.sock")
}

const socketPath = defaultSocketPath()

const cliArgs = process.argv.slice(2)
let currentWorkspace = process.env.TRSS_WORKSPACE_NAME || "default"
for (let index = 0; index < cliArgs.length; index++) {
  if (cliArgs[index] === "--workspace" && cliArgs[index + 1]) {
    currentWorkspace = safeWorkspaceName(cliArgs[++index])
  }
}
currentWorkspace = safeWorkspaceName(currentWorkspace)

const renderer = await createCliRenderer({
  exitOnCtrlC: false,
  useMouse: true,
  enableMouseMovement: false,
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
  content: " TRSS",
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
  content: " ↑↓ seleccionar",
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

const terminalArea = new BoxRenderable(renderer, {
  id: "terminal-area",
  flexGrow: 1,
  flexShrink: 1,
  height: "100%",
  backgroundColor: RGBA.fromInts(15, 17, 22),
})

body.add(hostPanel)
body.add(terminalArea)

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
let newHostVisible = false
let newHostForm: {
  field: number
  alias: string
  hostname: string
  port: string
  username: string
  identityFile: string
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
let actionBarFocused = false
let actionBarCursor = 0
let actionsMenuVisible = false
let actionsMenuCursor = 0

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
addActionButton("Prefs", () => { preferencesVisible = true })
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
  { label: "Preferencias", run: () => { preferencesVisible = true } },
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
  })
  actionsMenuBox.add(row)
  actionsMenuRows.push(row)
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
  "Salir de TRSS",
  "Ayuda",
]

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
  return `trss-${safe || "host"}`
}

function filteredHosts(): HostProfile[] {
  const needle = query.trim().toLowerCase()
  if (!needle) return hosts
  return hosts.filter((host) =>
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
  )
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
  hostTitle.content = ` Hosts (${all.length}${query ? `/${hosts.length}` : ""})${conflicts > 0 ? ` !${conflicts}` : ""}`
  hostQuery.content = query ? ` /${truncate(query, 24)}` : " ↑↓ seleccionar"
  hostHint.content =
    hosts.length === 0
      ? " Ctrl+N nuevo host · Ctrl+I KeePass"
      : " Enter conectar · Ctrl+N nuevo · Ctrl+I KeePass"

  if (hosts.length === 0) {
    hostList.content =
      `\n No hay hosts disponibles.\n\n` +
      ` trssd: ${socketPath}\n\n` +
      " Añade un bloque Host a ~/.ssh/config\n"
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
  if (cursor < scroll) scroll = cursor
  if (cursor >= scroll + rows) scroll = cursor - rows + 1

  const lines: string[] = []
  for (let index = scroll; index < Math.min(all.length, scroll + rows); index++) {
    const host = all[index]
    const session = sessionForHost(host)
    const tab = session ? store.list().find((item) => item.sessionId === session.sessionId) : undefined
    const pending = !session && pendingHostFor(host) !== undefined
    const state = tab ? phaseIcon(tab.phase) : pending ? "○" : " "
    const favorite = host.favorite ? "★" : " "
    const group = host.labels && host.labels.length > 0 ? ` (${host.labels[0]})` : ""
    const label = `${favorite}${host.alias}${group}`
    lines.push(`${index === cursor ? ">" : " "} ${state} ${truncate(label, hostPanelWidth - 4)}`)
    if (preferences.density === "comfortable") lines.push("")
  }
  hostList.content = lines.join("\n")
}

function renderTabs(): void {
  tabBar.content = renderTabBarStyled({
    tabs: store.list(),
    activeId: store.active()?.id,
    width: renderer.width,
  })
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
      ` Hosts · ↑↓ Enter conectar · Ctrl+N nuevo · F2 favorito · F3 color · F4 etiquetas` +
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
  renderStatus()
  renderWorkspacePicker()
  renderPalette()
  renderCloseDialog()
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

function renderCloseDialog(): void {
  if (!closeDialogVisible) {
    closeOverlay.visible = false
    return
  }
  closeOverlay.visible = true
  const choices = closeChoices()
  closeDialogCursor = Math.min(closeDialogCursor, choices.length - 1)
  const lines = ["", "  Cerrar pestaña", ""]
  for (let index = 0; index < choices.length; index++) {
    lines.push(`  ${index === closeDialogCursor ? ">" : " "} ${choices[index].label}`)
  }
  lines.push("", "  Enter confirmar · Esc cancelar")
  closeOverlay.content = lines.join("\n")
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
  closeDialogVisible = false
  void closeActive(choice.intent)
}

function handleCloseDialogKey(key: KeyEvent): void {
  const name = key.name.toLowerCase()
  if (name === "up") closeDialogCursor = Math.max(0, closeDialogCursor - 1)
  else if (name === "down") {
    closeDialogCursor = Math.min(closeChoices().length - 1, closeDialogCursor + 1)
  } else if (name === "escape") {
    closeCloseDialog()
    return
  } else if (name === "return" || name === "enter") {
    executeCloseDialog()
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
  const lines = ["", "  Comandos", ""]
  for (let index = 0; index < paletteCommands.length; index++) {
    const marker = index === paletteCursor ? ">" : " "
    lines.push(`  ${marker} ${paletteCommands[index]}`)
  }
  lines.push("", "  Enter ejecutar · Esc cerrar")
  paletteOverlay.content = lines.join("\n")
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
      preferencesVisible = true
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
  if (name === "up") paletteCursor = Math.max(0, paletteCursor - 1)
  else if (name === "down") paletteCursor = Math.min(paletteCommands.length - 1, paletteCursor + 1)
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
  const cols = Math.max(20, Math.floor(terminalArea.width) || renderer.width - hostPanelWidth)
  const rows = Math.max(5, renderer.height - 2)
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
    const labels = labelsInput.split(/\s+/).filter(Boolean)
    const hostID = labelsHostID
    labelsEditing = false
    await applyHostOverride(hostID, { labels })
  } else if (name === "backspace" || name === "delete") {
    labelsInput = name === "delete" ? "" : labelsInput.slice(0, -1)
  } else if (!key.ctrl && !key.meta && key.sequence) {
    const chars = Array.from(key.sequence)
    if (chars.length === 1 && chars[0] >= " ") labelsInput += chars[0]
  }
  uiDirty()
}

function handleHostKey(key: KeyEvent): void {
  const name = key.name.toLowerCase()
  const all = filteredHosts()

  if (matchesAction(key, keybindings, "favorite")) void toggleFavorite()
  else if (matchesAction(key, keybindings, "color")) void cycleHostColor()
  else if (matchesAction(key, keybindings, "labels")) openLabelsEditor()
  else if (name === "up") cursor = Math.max(0, cursor - 1)
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
  } else if (!key.ctrl && !key.meta && key.sequence) {
    const chars = Array.from(key.sequence)
    if (chars.length === 1 && chars[0] >= " ") {
      query += chars[0]
      cursor = 0
    }
  }
  uiDirty()
}

renderer.keyInput.on("keypress", (key) => {
  const name = key.name.toLowerCase()
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
    uiDirty()
    return
  }
  if (actionBarFocused) {
    key.stopPropagation()
    if (name === "left" || name === "backtab") actionBarCursor = (actionBarCursor + actionBarButtons.length - 1) % actionBarButtons.length
    else if (name === "right" || name === "tab") actionBarCursor = (actionBarCursor + 1) % actionBarButtons.length
    else if (name === "escape") actionBarFocused = false
    else if (name === "return" || name === "enter") actionBarButtons[actionBarCursor]?.run()
    uiDirty()
    return
  }
  if (hostFocused && name === "tab") {
    key.stopPropagation()
    actionBarFocused = true
    actionBarCursor = 0
    uiDirty()
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
  if (hostFocused) {
    key.stopPropagation()
    handleHostKey(key)
  }
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
  const lines = [
    "",
    "  Workspaces",
    "",
  ]
  if (workspaceNewMode) {
    lines.push(`  Nuevo: ${workspaceInput}▌`, "", "  Enter crear/abrir · Esc volver")
  } else {
    for (let index = 0; index < workspaceNames.length; index++) {
      const marker = index === workspacePickerCursor ? ">" : " "
      const current = workspaceNames[index] === currentWorkspace ? "  (actual)" : ""
      lines.push(`  ${marker} ${workspaceNames[index]}${current}`)
    }
    const marker = workspacePickerCursor === workspaceNames.length ? ">" : " "
    lines.push(`  ${marker} + Nuevo workspace…`, "", "  Enter abrir · n nuevo · Esc cerrar")
  }
  workspaceOverlay.content = lines.join("\n")
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

  if (name === "up") workspacePickerCursor = Math.max(0, workspacePickerCursor - 1)
  else if (name === "down") {
    workspacePickerCursor = Math.min(workspaceNames.length, workspacePickerCursor + 1)
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
    const selected = actionBarFocused && index === actionBarCursor
    button.node.fg = selected ? RGBA.fromInts(15, 18, 24) : RGBA.fromInts(215, 222, 235)
    button.node.bg = selected ? RGBA.fromInts(120, 190, 255) : RGBA.fromInts(38, 44, 58)
  }
}

function renderActionsMenu(): void {
  actionsMenuBox.visible = actionsMenuVisible
  if (!actionsMenuVisible) return
  actionsMenuBox.height = Math.min(actionsMenuItems.length + 5, 33)
  for (let index = 0; index < actionsMenuRows.length; index++) {
    const selected = index === actionsMenuCursor
    actionsMenuRows[index].content = `  ${selected ? ">" : " "} ${actionsMenuItems[index].label}`
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
  else if (name === "up") actionsMenuCursor = Math.max(0, actionsMenuCursor - 1)
  else if (name === "down") actionsMenuCursor = Math.min(actionsMenuItems.length - 1, actionsMenuCursor + 1)
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

function renderNewHost(): void {
  if (!newHostVisible || !newHostForm) {
    newHostOverlay.visible = false
    return
  }
  newHostOverlay.visible = true
  const form = newHostForm
  const marker = (index: number) => (index === form.field ? ">" : " ")
  newHostOverlay.content = [
    "",
    "  Nuevo host",
    "",
    `  ${marker(0)} Alias:        ${form.alias}`,
    `  ${marker(1)} HostName:    ${form.hostname}`,
    `  ${marker(2)} Puerto:      ${form.port}`,
    `  ${marker(3)} Usuario:     ${form.username}`,
    `  ${marker(4)} IdentityFile: ${form.identityFile}`,
    "",
    "  Tab/↑↓ cambiar campo · Enter guardar y conectar · Esc cancelar",
  ].join("\n")
}

function openNewHost(): void {
  newHostVisible = true
  newHostForm = {
    field: 0,
    alias: "",
    hostname: "",
    port: "22",
    username: process.env.USER ?? "",
    identityFile: "",
  }
  uiDirty()
}

async function handleNewHostKey(key: KeyEvent): Promise<void> {
  if (!newHostForm) return
  const form = newHostForm
  const name = key.name.toLowerCase()
  if (name === "escape") {
    newHostVisible = false
    newHostForm = null
  } else if (name === "tab" || name === "down") {
    form.field = (form.field + 1) % 5
  } else if (name === "up") {
    form.field = (form.field + 4) % 5
  } else if (name === "return" || name === "enter") {
    await saveNewHost(true)
    return
  } else if (name === "backspace" || name === "delete") {
    const clear = name === "delete"
    if (form.field === 0) form.alias = clear ? "" : form.alias.slice(0, -1)
    else if (form.field === 1) form.hostname = clear ? "" : form.hostname.slice(0, -1)
    else if (form.field === 2) form.port = clear ? "" : form.port.slice(0, -1)
    else if (form.field === 3) form.username = clear ? "" : form.username.slice(0, -1)
    else if (form.field === 4) form.identityFile = clear ? "" : form.identityFile.slice(0, -1)
  } else if (!key.ctrl && !key.meta && key.sequence) {
    const chars = Array.from(key.sequence)
    if (chars.length === 1 && chars[0] >= " ") {
      if (form.field === 0) form.alias += chars[0]
      else if (form.field === 1) form.hostname += chars[0]
      else if (form.field === 2 && /[0-9]/.test(chars[0])) form.port += chars[0]
      else if (form.field === 3) form.username += chars[0]
      else if (form.field === 4) form.identityFile += chars[0]
    }
  }
  uiDirty()
}

async function saveNewHost(connect: boolean): Promise<void> {
  if (!client || !newHostForm) return
  const form = newHostForm
  const port = Number.parseInt(form.port, 10)
  if (!form.alias.trim() || !form.hostname.trim()) {
    transientText = "Host: alias y HostName son obligatorios"
    uiDirty()
    return
  }
  if (!Number.isFinite(port) || port < 1 || port > 65535) {
    transientText = "Host: puerto inválido"
    uiDirty()
    return
  }
  try {
    const profile = await client.call(
      "hosts.create",
      {
        alias: form.alias,
        hostname: form.hostname,
        port,
        username: form.username,
        identityFile: form.identityFile,
      },
      { timeoutMs: 8000 },
    )
    await loadHosts()
    newHostVisible = false
    newHostForm = null
    transientText = `Host ${profile.alias} guardado`
    if (connect) await connectHost(profile)
    else uiDirty()
  } catch (error) {
    transientText = `No se pudo guardar el host: ${String(error)}`
    uiDirty()
  }
}

function renderHelp(): void {
  if (!helpVisible) {
    helpOverlay.visible = false
    return
  }
  helpOverlay.visible = true
  helpOverlay.content = [
    "",
    "  Ayuda · atajos",
    "",
    "  Ctrl+T hosts        Ctrl+K paleta        Ctrl+P workspaces",
    "  Ctrl+N nuevo host   Ctrl+G tmux          Ctrl+I KeePass       Ctrl+W cerrar",
    "  Ctrl+Tab pestaña    Ctrl+1..9 pestaña    Ctrl+Shift+←/→ mover",
    "  F2 favorito         F3 color             F4 etiquetas",
    "  F10 salir           F1 ayuda            F6 barra de acciones",
    "",
    "  En hosts: ↑↓ mover · Enter conectar · escribir busca",
    "  En FRP (Ctrl+F): n crear · d parar · x eliminar · s frps · r rotar",
    "  En transferencias: u subir · d descargar · c cancelar",
    "  En Nmap: Enter confirma el escaneo · Esc cancela",
    "",
    "  Esc/Enter cerrar",
  ].join("\n")
}

function renderNmapConfirm(): void {
  if (!nmapConfirmVisible) {
    nmapOverlay.visible = false
    return
  }
  nmapOverlay.visible = true
  const host = currentFormHost()
  nmapOverlay.content = [
    "",
    "  Escaneo Nmap optativo",
    "",
    `  Host: ${host?.alias ?? "(ninguno)"}`,
    `  Puertos: ${nmapPorts}▌`,
    "",
    "  Solo úsalo en redes autorizadas. Se ejecutará nmap -sT -Pn.",
    "  Enter escanear · Esc cancelar · edita los puertos",
  ].join("\n")
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
    const lines = [
      "",
      `  ${form.direction === "upload" ? "Subir" : "Descargar"} · ${form.protocol}`,
      "",
      `  ${form.field === 0 ? ">" : " "} Local:  ${form.localPath}`,
      `  ${form.field === 1 ? ">" : " "} Remoto: ${form.remotePath}`,
      "",
      `  Ctrl+P protocolo: ${form.protocol} · Ctrl+R recursivo: ${form.recursive ? "sí" : "no"}`,
      "",
      "  Tab/↑↓ campo · Enter iniciar · Esc cancelar",
    ]
    transferOverlay.content = lines.join("\n")
    return
  }
  const lines = ["", "  Transferencias", ""]
  if (transfers.length === 0) lines.push("  Sin transferencias")
  for (let index = 0; index < Math.min(transfers.length, 8); index++) {
    const item = transfers[index]
    const marker = index === transferCursor ? ">" : " "
    const arrow = item.direction === "upload" ? "→" : "←"
    const error = item.errorLabel ? ` · ${item.errorLabel}` : ""
    lines.push(
      `  ${marker} ${item.state} ${item.protocol} ${item.localPath} ${arrow} ${item.remotePath}${error}`,
    )
  }
  lines.push("", "  u subir · d descargar · c cancelar · Esc cerrar")
  transferOverlay.content = lines.join("\n")
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
  if (transferForm) {
    const form = transferForm
    if (name === "escape") {
      transferForm = null
    } else if (name === "tab" || name === "down") {
      form.field = (form.field + 1) % 2
    } else if (name === "up") {
      form.field = (form.field + 1) % 2
    } else if (key.ctrl && name === "p") {
      form.protocol = form.protocol === "sftp" ? "scp" : "sftp"
    } else if (key.ctrl && name === "r") {
      form.recursive = !form.recursive
    } else if (name === "backspace" || name === "delete") {
      if (form.field === 0) {
        form.localPath = name === "delete" ? "" : form.localPath.slice(0, -1)
      } else {
        form.remotePath = name === "delete" ? "" : form.remotePath.slice(0, -1)
      }
    } else if (name === "return" || name === "enter") {
      await startTransferForm()
      return
    } else if (!key.ctrl && !key.meta && key.sequence) {
      const chars = Array.from(key.sequence)
      if (chars.length === 1 && chars[0] >= " ") {
        if (form.field === 0) form.localPath += chars[0]
        else form.remotePath += chars[0]
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
  } else if (name === "up") {
    transferCursor = Math.max(0, transferCursor - 1)
  } else if (name === "down") {
    transferCursor = Math.min(Math.max(0, transfers.length - 1), transferCursor + 1)
  } else if (name === "c" && transfers[transferCursor] && client) {
    try {
      await client.call(
        "transfers.cancel",
        { transferId: transfers[transferCursor].transferId },
        { timeoutMs: 5000 },
      )
      await refreshTransfers()
    } catch (error) {
      transientText = `Cancelar transferencia: ${String(error)}`
    }
  }
  uiDirty()
}

function renderPreferences(): void {
  if (!preferencesVisible) {
    prefsOverlay.visible = false
    return
  }
  prefsOverlay.visible = true
  prefsOverlay.content = [
    "",
    "  Preferencias",
    "",
    `  w/←/→  ancho del panel: ${preferences.hostPanelWidth}`,
    `  d      densidad: ${preferences.density}`,
    `  t      posición de pestañas: ${preferences.tabPosition}`,
    "",
    "  Esc/Enter cerrar",
  ].join("\n")
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
  const cols = Math.max(20, renderer.width - hostPanelWidth)
  const rows = Math.max(5, renderer.height - 2)
  for (const view of sessions.values()) {
    scheduleResize(view.sessionId, cols, view.terminal.height || rows)
  }
}

function handlePreferencesKey(key: KeyEvent): void {
  const name = key.name.toLowerCase()
  if (name === "escape" || name === "return" || name === "enter") {
    preferencesVisible = false
  } else if (name === "w" || name === "right") {
    preferences.hostPanelWidth = nextPanelWidth(preferences.hostPanelWidth, 1)
    hostPanelWidth = preferences.hostPanelWidth
    hostPanel.width = hostPanelWidth
    persistPreferences()
    resizeTerminals()
  } else if (name === "left") {
    preferences.hostPanelWidth = nextPanelWidth(preferences.hostPanelWidth, -1)
    hostPanelWidth = preferences.hostPanelWidth
    hostPanel.width = hostPanelWidth
    persistPreferences()
    resizeTerminals()
  } else if (name === "d") {
    preferences.density = preferences.density === "compact" ? "comfortable" : "compact"
    persistPreferences()
  } else if (name === "t") {
    preferences.tabPosition = preferences.tabPosition === "top" ? "bottom" : "top"
    applyTabPosition()
    persistPreferences()
  }
  uiDirty()
}

function renderDiagnostics(): void {
  if (!diagVisible) {
    diagOverlay.visible = false
    return
  }
  diagOverlay.visible = true
  diagOverlay.content = diagContent
}

function renderLabelsEditor(): void {
  if (!labelsEditing) {
    labelsOverlay.visible = false
    return
  }
  labelsOverlay.visible = true
  labelsOverlay.content = [
    "",
    "  Etiquetas del host",
    "",
    `  ${labelsInput}▌`,
    "",
    "  Enter guardar · Esc cancelar",
  ].join("\n")
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
      `  Nombre: ${form.name}`,
      `  Tipo: ${form.type}  (t/←/→)`,
      `  Service port: ${form.servicePort}`,
      `  Remote port: ${form.remotePort}`,
      `  Bind port (visitor STCP/XTCP): ${form.bindPort}`,
      `  Exposición: ${form.type === "tcp" || form.type === "udp" ? "PÚBLICA en frps" : "privada (visitor)"}`,
    ]
    const lines = ["", "  Nuevo proxy FRP", ""]
    fields.forEach((line, index) => lines.push(`  ${index === form.field ? ">" : " "}${line}`))
    lines.push("", "  Tab/↑↓ campo · t tipo · Enter aplicar · Esc cancelar")
    frpOverlay.content = lines.join("\n")
    return
  }
  const lines = ["", "  FRP · endpoints", ""]
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
  if (frpProxies.length === 0) lines.push("  Sin proxies configurados")
  for (let index = 0; index < frpProxies.length; index++) {
    const proxy = frpProxies[index]
    const marker = index === frpCursor ? ">" : " "
    const target =
      proxy.type === "stcp" || proxy.type === "xtcp"
        ? proxy.bindPort
          ? `visitor 127.0.0.1:${proxy.bindPort}`
          : `stcp ${proxy.servicePort}`
        : `${proxy.servicePort} → ${proxy.remotePort}`
    const stats = proxy.pid ? ` pid=${proxy.pid} up=${proxy.uptimeSeconds ?? 0}s` : ""
    const error = proxy.lastErrorLabel ? ` · ${proxy.lastErrorLabel}` : ""
    lines.push(`  ${marker} ${proxy.state} ${proxy.name} [${proxy.type}] ${target}${stats}${error}`)
  }
  lines.push("", "  n nuevo · d parar · x eliminar · s iniciar/parar frps · r rotar token · Esc cerrar")
  frpOverlay.content = lines.join("\n")
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
  if (frpForm) {
    const form = frpForm
    if (name === "escape") {
      frpForm = null
    } else if (name === "tab" || name === "down") {
      form.field = (form.field + 1) % 5
    } else if (name === "up") {
      form.field = (form.field + 4) % 5
    } else if (name === "t" || name === "left" || name === "right") {
      form.type = nextFRPFormType(form.type)
    } else if (name === "backspace") {
      if (form.field === 0) form.name = form.name.slice(0, -1)
      else if (form.field === 2) form.servicePort = form.servicePort.slice(0, -1)
      else if (form.field === 3) form.remotePort = form.remotePort.slice(0, -1)
      else if (form.field === 4) form.bindPort = form.bindPort.slice(0, -1)
    } else if (name === "return" || name === "enter") {
      await applyFrpForm()
      return
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
  } else if (name === "up") {
    frpCursor = Math.max(0, frpCursor - 1)
    frpDeleteArmed = ""
  } else if (name === "down") {
    frpCursor = Math.min(Math.max(0, frpProxies.length - 1), frpCursor + 1)
    frpDeleteArmed = ""
  } else if (name === "n") {
    const host = currentFormHost()
    frpForm = {
      field: 0,
      name: host ? `proxy-${host.alias}` : "proxy",
      type: "tcp",
      servicePort: host ? String(host.target.port) : "22",
      remotePort: "",
      bindPort: "",
    }
  } else if (name === "x" && frpProxies[frpCursor] && client) {
    const proxy = frpProxies[frpCursor]
    if (frpDeleteArmed !== proxy.id) {
      frpDeleteArmed = proxy.id
      transientText = `FRP: pulsa x otra vez para eliminar ${proxy.name}`
    } else {
      try {
        await client.call("frp.proxies.remove", { proxyId: proxy.id }, { timeoutMs: 8000 })
        frpDeleteArmed = ""
        await refreshFRP()
        transientText = `FRP: ${proxy.name} eliminado`
      } catch (error) {
        transientText = `FRP remove: ${String(error)}`
      }
    }
  } else if (name === "d" && frpProxies[frpCursor] && client) {
    try {
      await client.call("frp.proxies.stop", { proxyId: frpProxies[frpCursor].id }, { timeoutMs: 5000 })
      await refreshFRP()
    } catch (error) {
      transientText = `FRP stop: ${String(error)}`
    }
  } else if (name === "s" && client) {
    const endpoint = frpEndpoints.find((item) => item.managed)
    if (!endpoint) {
      transientText = "FRP: no hay frps local gestionado"
    } else {
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
      }
    }
  } else if (name === "r" && frpEndpoints[0] && client) {
    try {
      await client.call("frp.tokens.rotate", { endpointId: frpEndpoints[0].id }, { timeoutMs: 15_000 })
      await refreshFRP()
      transientText = "FRP: token rotado"
    } catch (error) {
      transientText = `FRP rotate: ${String(error)}`
    }
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

async function start(): Promise<void> {
  uiDirty()
  try {
    client = await IpcClient.connect(socketPath)
  } catch (error) {
    errorText = `No se pudo conectar a trssd en ${socketPath}: ${String(error)}. Arranca ./bin/trssd`
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
