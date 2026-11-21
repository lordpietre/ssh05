import { existsSync, readFileSync } from "node:fs"
import { homedir } from "node:os"
import { join } from "node:path"

export type KeyAction =
  | "hosts"
  | "palette"
  | "workspaces"
  | "frp"
  | "import"
  | "tmux"
  | "close"
  | "quit"

export type KeyBindings = Record<KeyAction, string>

export const DEFAULT_KEYBINDINGS: KeyBindings = {
  hosts: "ctrl+t",
  palette: "ctrl+k",
  workspaces: "ctrl+p",
  frp: "ctrl+f",
  import: "ctrl+i",
  tmux: "ctrl+g",
  close: "ctrl+w",
  quit: "f10",
}

export interface LoadedKeybindings {
  bindings: KeyBindings
  path: string
  errors: string[]
}

interface Chord {
  ctrl: boolean
  shift: boolean
  alt: boolean
  name: string
}

/** Parses `ctrl+shift+t`, `alt+1`, `f10` or `escape`. */
export function parseChord(value: string): Chord | undefined {
  const parts = value
    .split("+")
    .map((part) => part.trim().toLowerCase())
    .filter(Boolean)
  if (parts.length === 0) return undefined
  const chord: Chord = { ctrl: false, shift: false, alt: false, name: "" }
  for (const part of parts) {
    switch (part) {
      case "ctrl":
      case "control":
        chord.ctrl = true
        break
      case "shift":
        chord.shift = true
        break
      case "alt":
      case "meta":
        chord.alt = true
        break
      default:
        if (chord.name) return undefined
        chord.name = part
    }
  }
  return chord.name ? chord : undefined
}

interface KeyLike {
  name: string
  ctrl?: boolean
  shift?: boolean
  meta?: boolean
}

/** Matches a key event against a configured chord. */
export function matchesChord(key: KeyLike, chord: string | undefined): boolean {
  if (!chord) return false
  const parsed = parseChord(chord)
  if (!parsed) return false
  return (
    key.name.toLowerCase() === parsed.name &&
    Boolean(key.ctrl) === parsed.ctrl &&
    Boolean(key.shift) === parsed.shift &&
    Boolean(key.meta) === parsed.alt
  )
}

export function matchesAction(key: KeyLike, bindings: KeyBindings, action: KeyAction): boolean {
  return matchesChord(key, bindings[action])
}

/**
 * Loads bindings from `TRSS_KEYBINDINGS` or
 * `~/.config/trss/keybindings.json`. Invalid entries keep the default and are
 * reported in `errors` instead of breaking the UI.
 */
export function loadKeybindings(
  env: Record<string, string | undefined> = process.env,
  readFile: (path: string) => string = (path) => readFileSync(path, "utf8"),
  exists: (path: string) => boolean = existsSync,
): LoadedKeybindings {
  const path = env.TRSS_KEYBINDINGS || join(homedir(), ".config", "trss", "keybindings.json")
  const bindings = { ...DEFAULT_KEYBINDINGS }
  const errors: string[] = []
  if (!exists(path)) return { bindings, path, errors }
  let data: unknown
  try {
    data = JSON.parse(readFile(path))
  } catch (error) {
    errors.push(`keybindings: ${String(error)}`)
    return { bindings, path, errors }
  }
  if (!data || typeof data !== "object" || Array.isArray(data)) {
    errors.push("keybindings: expected a JSON object")
    return { bindings, path, errors }
  }
  for (const [rawAction, rawChord] of Object.entries(data as Record<string, unknown>)) {
    const action = rawAction as KeyAction
    if (!(action in DEFAULT_KEYBINDINGS)) {
      errors.push(`keybindings: unknown action ${rawAction}`)
      continue
    }
    if (rawChord === null || rawChord === "") {
      bindings[action] = ""
      continue
    }
    if (typeof rawChord !== "string" || !parseChord(rawChord)) {
      errors.push(`keybindings: invalid chord for ${rawAction}`)
      continue
    }
    bindings[action] = rawChord
  }
  const seen = new Map<string, KeyAction>()
  for (const action of Object.keys(bindings) as KeyAction[]) {
    const chord = bindings[action]
    if (!chord) continue
    const previous = seen.get(chord.toLowerCase())
    if (previous) {
      errors.push(`keybindings: ${action} duplicates ${previous}`)
      bindings[action] = DEFAULT_KEYBINDINGS[action]
      continue
    }
    seen.set(chord.toLowerCase(), action)
  }
  return { bindings, path, errors }
}
