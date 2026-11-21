/**
 * TRSS Phase 0 prototype: OpenTUI host TUI + Bun PTY + embedded terminal.
 *
 * This is intentionally small. It proves the integration surface described in
 * TRSS_SDD_v0.2 §17:
 *   - EmbeddedTerminalRenderable interprets ANSI and keeps a VT screen.
 *   - Bun.spawn({ terminal }) gives us a real PTY and SIGWINCH on resize.
 *   - Keyboard input and resize are forwarded without a second emulator.
 *
 * Usage:
 *   bun run src/main.ts                 # $SHELL
 *   bun run src/main.ts -- vim /tmp/x   # any interactive command
 */

import {
  BoxRenderable,
  EmbeddedTerminalRenderable,
  RGBA,
  TextRenderable,
  createCliRenderer,
} from "@opentui/core"
import { appendFileSync } from "node:fs"

const debugFile = process.env.TRSS_PROTO_DEBUG
const debug = (message: string) => {
  if (debugFile) appendFileSync(debugFile, `${new Date().toISOString()} ${message}\n`)
}

const command = process.argv.slice(2)
if (command[0] === "--") command.shift()
const argv = command.length > 0 ? command : [process.env.SHELL || "/bin/sh"]

const renderer = await createCliRenderer({
  exitOnCtrlC: false,
  useMouse: true,
  enableMouseMovement: false,
})

let pty: Bun.Subprocess | undefined

const root = new BoxRenderable(renderer, {
  id: "root",
  width: "100%",
  height: "100%",
  flexDirection: "column",
  backgroundColor: RGBA.fromInts(16, 18, 24),
})

const initialCols = Math.max(20, renderer.width)
const initialRows = Math.max(5, renderer.height - 2)

const tabBar = new TextRenderable(renderer, {
  id: "tabbar",
  content: ` TRSS · Phase 0 prototype   [1 ${argv.join(" ")}]   F10 quit`,
  fg: RGBA.fromInts(220, 225, 235),
  bg: RGBA.fromInts(38, 44, 58),
  height: 1,
})

const terminal = new EmbeddedTerminalRenderable(renderer, {
  id: "terminal",
  width: "100%",
  flexGrow: 1,
  flexShrink: 1,
  cols: initialCols,
  rows: initialRows,
  maxScrollback: 10_000,
  selectable: true,
  onData: (data, source) => {
    if (source === "input" || source === "response") {
      pty?.terminal?.write(data)
    }
  },
  onTerminalResize: (cols, rows) => {
    debug(`terminal resize ${cols}x${rows}`)
    if (cols > 0 && rows > 0) {
      pty?.terminal?.resize(cols, rows)
    }
  },
})

const status = new TextRenderable(renderer, {
  id: "status",
  content: " starting…",
  fg: RGBA.fromInts(160, 170, 185),
  bg: RGBA.fromInts(24, 27, 35),
  height: 1,
})

root.add(tabBar)
root.add(terminal)
root.add(status)
renderer.root.add(root)
renderer.start()
renderer.on("resize", (width: number, height: number) => {
  debug(`renderer resize event ${width}x${height}`)
})

terminal.focus()

process.on("SIGWINCH", () => {
  const cols = process.stdout.columns
  const rows = process.stdout.rows
  debug(
    `SIGWINCH stdout=${cols}x${rows} ` +
      `renderer=${renderer.width}x${renderer.height} terminal=${terminal.width}x${terminal.height}`,
  )
  // Bun 1.4.5 refreshes stdout.columns on SIGWINCH, but OpenTUI's own debounced
  // path can be lost while a native frame is in flight. Drive the documented
  // external-resize API directly; processResize() also handles OutputBusy.
  if (cols > 0 && rows > 0) {
    try {
      renderer.resize(cols, rows)
      debug(`resize call done renderer=${renderer.width}x${renderer.height}`)
    } catch (error) {
      debug(`resize call failed: ${String(error)}`)
    }
  }
})

pty = Bun.spawn({
  cmd: argv,
  env: { ...process.env, TERM: "xterm-256color" },
  terminal: {
    cols: initialCols,
    rows: initialRows,
    name: "xterm-256color",
    data: (_term, data) => {
      terminal.write(data)
    },
    exit: (_term, code) => {
      status.content = ` PTY closed (${code})`
      renderer.requestRender()
    },
  },
})

status.content = ` running: ${argv.join(" ")} (pid ${pty.pid})`
renderer.requestRender()

void pty.exited.then((code) => {
  // Let the last frame render, then leave the alternate screen cleanly.
  setTimeout(() => {
    renderer.destroy()
    process.exit(code ?? 0)
  }, 150)
})

renderer.keyInput.on("keypress", (key) => {
  const isQuit =
    key.name.toLowerCase() === "f10" ||
    (key.ctrl && key.shift && key.name.toLowerCase() === "q") ||
    (key.ctrl && key.name.toLowerCase() === "q" && process.env.TRSS_PROTO_ALLOW_CTRL_Q === "1")
  if (isQuit) {
    const child = pty
    child?.terminal?.close()
    child?.kill()
    renderer.destroy()
    process.exit(0)
  }
})
