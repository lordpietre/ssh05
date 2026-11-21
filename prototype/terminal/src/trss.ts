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
  HostProfile,
  IpcEvent,
  SessionId,
  SessionInfo,
  SessionPhase,
  SessionStatus,
  TabId,
  TabRecord,
  TabState,
  Workspace,
} from "../../../contracts/ts/index.ts"
import { IpcClient } from "./ipc/client.ts"
import { InMemoryTabStore } from "./tabs/store.ts"
import { phaseIcon, phaseLabel, renderTabBar } from "./tabs/tabbar.ts"
import { loadWorkspace, saveWorkspace } from "./workspaces/store.ts"

const HOST_PANEL_WIDTH = 30
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

function defaultSocketPath(): string {
  if (process.env.TRSS_SOCKET) return process.env.TRSS_SOCKET
  const runtimeDir = process.env.XDG_RUNTIME_DIR
  if (runtimeDir) return join(runtimeDir, "trss", "control.sock")
  const uid = typeof process.getuid === "function" ? process.getuid() : "user"
  return join(tmpdir(), `trss-${uid}`, "control.sock")
}

const socketPath = defaultSocketPath()

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
  width: HOST_PANEL_WIDTH,
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
renderer.start()

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
    `${host.alias} ${host.target.hostname} ${host.target.username ?? ""} ${host.target.port}`
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
  hostHint.content = hosts.length === 0 ? " Ctrl+I KeePass · revisa trssd" : " Enter conectar · Ctrl+I KeePass"

  if (hosts.length === 0) {
    hostList.content =
      `\n No hay hosts disponibles.\n\n` +
      ` trssd: ${socketPath}\n\n` +
      " Añade un bloque Host a ~/.ssh/config\n"
    return
  }

  const rows = Math.max(1, Math.floor(hostList.height) || Math.max(1, renderer.height - 7))
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
    lines.push(`${index === cursor ? ">" : " "} ${state} ${truncate(host.alias, HOST_PANEL_WIDTH - 5)}`)
  }
  hostList.content = lines.join("\n")
}

function renderTabs(): void {
  tabBar.content = renderTabBar({
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
    status.content = ` Hosts · ↑↓ Enter conectar · Ctrl+I KeePass · Esc volver · tmux:${tmuxMode ? "on" : "off"} · F10 salir`
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
    ` · tmux:${active.tmux ? "on" : "off"} · Ctrl+Tab cambiar · Ctrl+W cerrar`
}

function uiDirty(): void {
  renderHosts()
  renderTabs()
  renderStatus()
  renderer.requestRender()
}

function snapshotWorkspace(): Workspace {
  const tabs: TabRecord[] = store.list().map((tab, index) => {
    const view = sessions.get(tab.sessionId)
    const pending = pendingHosts.get(tab.id)
    return {
      id: tab.id,
      title: tab.title,
      hostProfileId: view?.host.id ?? pending?.id ?? tab.title,
      tmux: tab.tmux,
      color: tab.color,
      order: index,
    }
  })
  return {
    id: "default",
    name: "default",
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
      saveWorkspace(snapshotWorkspace())
    } catch (error) {
      debug(`workspace save: ${String(error)}`)
    }
  }, 150)
}

async function restoreWorkspace(liveSessions: SessionInfo[]): Promise<void> {
  const workspace = loadWorkspace()
  const records = workspace ? [...workspace.tabs].sort((a, b) => a.order - b.order) : []
  const used = new Set<SessionId>()
  restoringWorkspace = true
  try {
    for (const record of records) {
      const host = hosts.find(
        (item) => item.id === record.hostProfileId || item.alias === record.hostProfileId,
      )
      if (!host) continue

      const live = liveSessions.find(
        (info) => !used.has(info.sessionId) && info.profileId === host.id,
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
        color: record.color,
        tmux: record.tmux,
      })
      pendingHosts.set(id, host)
    }

    // Live sessions that were not in the workspace are still owned by the
    // supervisor; expose them instead of leaving unreachable orphans.
    for (const live of liveSessions) {
      if (used.has(live.sessionId)) continue
      const host = hosts.find((item) => item.id === live.profileId || item.alias === live.profileId)
      if (!host) continue
      try {
        await attachExisting(live, host)
      } catch (error) {
        debug(`attach orphan ${live.sessionId}: ${String(error)}`)
      }
    }

    const preferred = workspace?.preferences?.activeTabId
    const activeId =
      preferred && store.list().some((tab) => tab.id === preferred)
        ? preferred
        : store.list()[0]?.id
    if (activeId) activate(activeId)
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
    if (tab) store.update(tab.id, { phase: status.phase })
    pendingStatus.delete(sessionId)
  }
}

function createTerminalView(sessionId: SessionId, host: HostProfile, tmuxSession?: string): SessionView {
  const cols = Math.max(20, Math.floor(terminalArea.width) || renderer.width - HOST_PANEL_WIDTH)
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
  const view: SessionView = { sessionId, host, terminal }
  sessions.set(sessionId, view)
  return view
}

function tabForSession(sessionId: SessionId, host: HostProfile, phase: SessionPhase, tmuxSession?: string): TabState {
  return {
    id: sessionId,
    sessionId,
    title: host.alias,
    phase,
    tmux: tmuxSession ? { hostProfileId: host.id, sessionName: tmuxSession, verified: true } : undefined,
  }
}

async function connectHost(
  host: HostProfile,
  options: { tmuxSession?: string; replaceTabId?: TabId } = {},
): Promise<void> {
  if (!client || quitting) return
  if (sessions.size >= MAX_SESSIONS) {
    transientText = `Máximo de ${MAX_SESSIONS} sesiones alcanzado`
    uiDirty()
    return
  }
  transientText = ""
  errorText = ""
  const tmuxSession = options.tmuxSession ?? (tmuxMode ? tmuxSessionName(host.alias) : undefined)
  try {
    const opened = await client.call(
      "sessions.open",
      { hostProfileId: host.id, tmuxSession },
      { timeoutMs: 20_000 },
    )
    if (options.replaceTabId) {
      pendingHosts.delete(options.replaceTabId)
      store.remove(options.replaceTabId)
    }
    const view = createTerminalView(opened.sessionId, host, tmuxSession)
    const tab = tabForSession(opened.sessionId, host, "connected", tmuxSession)
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
  const view = createTerminalView(info.sessionId, host, info.tmuxSession)
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

async function closeActive(): Promise<void> {
  const active = store.active()
  if (!active) return
  const view = sessions.get(active.sessionId)
  if (view && client) {
    try {
      await client.call(
        "sessions.close",
        { sessionId: active.sessionId, intent: "close-view" },
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

function reconnectActive(): void {
  const active = store.active()
  if (!active) return
  const host = pendingHosts.get(active.id)
  if (!host) return
  void connectHost(host, {
    replaceTabId: active.id,
    tmuxSession: active.tmux?.sessionName,
  })
}

async function quit(): Promise<void> {
  if (quitting) return
  if (saveTimer) {
    clearTimeout(saveTimer)
    saveTimer = undefined
  }
  try {
    saveWorkspace(snapshotWorkspace())
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

function handleHostKey(key: KeyEvent): void {
  const name = key.name.toLowerCase()
  const all = filteredHosts()

  if (name === "up") cursor = Math.max(0, cursor - 1)
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
  if (name === "f10") {
    key.stopPropagation()
    void quit()
    return
  }
  if (key.ctrl && name === "t") {
    key.stopPropagation()
    focusHosts()
    return
  }
  if (key.ctrl && name === "g") {
    key.stopPropagation()
    tmuxMode = !tmuxMode
    uiDirty()
    return
  }
  if (key.ctrl && name === "i") {
    key.stopPropagation()
    void importHosts()
    return
  }
  if (key.ctrl && name === "w") {
    key.stopPropagation()
    void closeActive()
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

async function loadHosts(): Promise<void> {
  if (!client) return
  const result = await client.call("hosts.list", {}, { timeoutMs: 5000 })
  hosts = [...result.hosts].sort((a, b) => a.alias.localeCompare(b.alias))
  conflicts = result.conflicts?.length ?? 0
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
    transientText = `KeePass: ${result.imported} hosts, ${result.conflicts?.length ?? 0} conflictos`
  } catch (error) {
    transientText = `Importar KeePass: ${String(error)}`
  }
  uiDirty()
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
    const tab = store.list().find((item) => item.sessionId === sessionId)
    if (tab) store.update(tab.id, { phase: status.phase })
    else pendingStatus.set(sessionId, status)
    uiDirty()
  })

  try {
    await loadHosts()
    const listed = await client
      .call("sessions.list", {}, { timeoutMs: 3000 })
      .catch(() => ({ sessions: [] as SessionInfo[] }))
    await restoreWorkspace(listed.sessions)
    uiDirty()
  } catch (error) {
    errorText = `hosts.list: ${String(error)}`
    uiDirty()
  }
}

void start()
