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
  SessionPhase,
  SessionStatus,
  TabId,
  TabState,
} from "../../../contracts/ts/index.ts"
import { IpcClient } from "./ipc/client.ts"
import { InMemoryTabStore } from "./tabs/store.ts"

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

let client: IpcClient | undefined
let hosts: HostProfile[] = []
let hostFocused = true
let cursor = 0
let scroll = 0
let query = ""
let quitting = false
let errorText = ""
let transientText = ""

function phaseIcon(phase: SessionPhase): string {
  switch (phase) {
    case "connecting":
    case "resolving":
    case "authenticating":
      return "↻"
    case "connected":
      return "●"
    case "degraded":
    case "reconnecting":
      return "!"
    case "disconnected":
    case "detached":
      return "○"
    case "failed":
      return "!"
    case "closing":
    case "closed":
      return "×"
    default:
      return "·"
  }
}

function phaseLabel(phase: SessionPhase): string {
  switch (phase) {
    case "connecting":
      return "conectando"
    case "resolving":
      return "resolviendo"
    case "authenticating":
      return "autenticando"
    case "connected":
      return "conectado"
    case "degraded":
      return "degradado"
    case "reconnecting":
      return "reconectando"
    case "disconnected":
      return "desconectado"
    case "detached":
      return "desacoplado"
    case "failed":
      return "error"
    case "closing":
      return "cerrando"
    case "closed":
      return "cerrado"
    default:
      return phase
  }
}

function truncate(value: string, max: number): string {
  if (value.length <= max) return value
  return `${value.slice(0, Math.max(0, max - 1))}…`
}

function hostTarget(host: HostProfile): string {
  const user = host.target.username ? `${host.target.username}@` : ""
  return `${user}${host.target.hostname}:${host.target.port}`
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

function renderHosts(): void {
  const all = filteredHosts()
  hostTitle.content = ` Hosts (${all.length}${query ? `/${hosts.length}` : ""})`
  hostQuery.content = query ? ` /${truncate(query, 24)}` : " ↑↓ seleccionar"
  hostHint.content = hosts.length === 0 ? " revisa trssd/ssh_config" : " Enter conectar"

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
    const state = tab ? phaseIcon(tab.phase) : " "
    lines.push(`${index === cursor ? ">" : " "} ${state} ${truncate(host.alias, HOST_PANEL_WIDTH - 5)}`)
  }
  hostList.content = lines.join("\n")
}

function renderTabs(): void {
  const tabs = store.list()
  if (tabs.length === 0) {
    tabBar.content = " TRSS · supervisor   Ctrl+T hosts   F10 salir"
    return
  }
  const active = store.active()
  const rendered = tabs.map((tab, index) => {
    const label = `${index + 1} ${phaseIcon(tab.phase)} ${truncate(tab.title, 18)}`
    return tab.id === active?.id ? `[${label}]` : ` ${label} `
  })
  tabBar.content = ` TRSS · ${rendered.join(" ")}  [+]`
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
    status.content = " Hosts · ↑↓ Enter conectar · / filtrar · Esc volver · F10 salir"
    return
  }
  const active = store.active()
  const view = active ? sessions.get(active.sessionId) : undefined
  if (!active || !view) {
    status.content = " Sin sesiones · Ctrl+T hosts · F10 salir"
    return
  }
  status.content =
    ` SSH ${phaseIcon(active.phase)} ${phaseLabel(active.phase)} · ${active.title} · ${hostTarget(view.host)}` +
    " · Ctrl+Tab cambiar · Ctrl+W cerrar"
}

function uiDirty(): void {
  renderHosts()
  renderTabs()
  renderStatus()
  renderer.requestRender()
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
  for (const [sessionId, view] of sessions) {
    view.terminal.visible = sessionId === active.sessionId
  }
  hostFocused = false
  sessions.get(active.sessionId)?.terminal.focus()
  uiDirty()
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

async function connectHost(host: HostProfile): Promise<void> {
  if (!client || quitting) return
  if (sessions.size >= MAX_SESSIONS) {
    transientText = `Máximo de ${MAX_SESSIONS} sesiones alcanzado`
    uiDirty()
    return
  }
  transientText = ""
  errorText = ""
  try {
    const opened = await client.call("sessions.open", { hostProfileId: host.id }, { timeoutMs: 20_000 })
    const cols = Math.max(20, Math.floor(terminalArea.width) || renderer.width - HOST_PANEL_WIDTH)
    const rows = Math.max(5, renderer.height - 2)
    const terminal = new EmbeddedTerminalRenderable(renderer, {
      id: `terminal-${opened.sessionId}`,
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
          client.sendBinary(opened.sessionId, data)
        }
      },
      onTerminalResize: (newCols, newRows) => {
        if (newCols > 0 && newRows > 0) scheduleResize(opened.sessionId, newCols, newRows)
      },
    })
    terminalArea.add(terminal)
    const view: SessionView = { sessionId: opened.sessionId, host, terminal }
    sessions.set(opened.sessionId, view)

    const tab: TabState = {
      id: opened.sessionId,
      sessionId: opened.sessionId,
      title: host.alias,
      phase: "connected",
    }
    store.add(tab)
    activate(tab.id)
    flushPending(opened.sessionId, view)
    scheduleResize(opened.sessionId, terminal.width || cols, terminal.height || rows)
  } catch (error) {
    transientText = `No se pudo abrir ${host.alias}: ${String(error)}`
    uiDirty()
  }
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
  if (!active || !client) return
  const view = sessions.get(active.sessionId)
  try {
    await client.call(
      "sessions.close",
      { sessionId: active.sessionId, intent: "close-view" },
      { timeoutMs: 5000 },
    )
  } catch (error) {
    transientText = `Cierre: ${String(error)}`
  }

  sessions.delete(active.sessionId)
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
  const next = store.active()
  if (next) activate(next.id)
  else focusHosts()
  uiDirty()
}

async function quit(): Promise<void> {
  if (quitting) return
  quitting = true
  const tabs = [...store.list()]
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
store.subscribe(() => uiDirty())

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
    const result = await client.call("hosts.list", {}, { timeoutMs: 5000 })
    hosts = [...result.hosts].sort((a, b) => a.alias.localeCompare(b.alias))
    uiDirty()
  } catch (error) {
    errorText = `hosts.list: ${String(error)}`
    uiDirty()
  }
}

void start()
