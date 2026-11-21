import { describe, expect, test } from "bun:test"
import {
  DEFAULT_KEYBINDINGS,
  loadKeybindings,
  matchesAction,
  matchesChord,
  parseChord,
} from "./keybindings.ts"

describe("parseChord", () => {
  test("parses modifiers and function keys", () => {
    expect(parseChord("Ctrl+Shift+T")).toEqual({ ctrl: true, shift: true, alt: false, name: "t" })
    expect(parseChord("f10")).toEqual({ ctrl: false, shift: false, alt: false, name: "f10" })
    expect(parseChord("ctrl+")).toBeUndefined()
    expect(parseChord("ctrl+a+b")).toBeUndefined()
  })
})

describe("matchesChord", () => {
  test("requires exact modifiers", () => {
    expect(matchesChord({ name: "t", ctrl: true }, "ctrl+t")).toBe(true)
    expect(matchesChord({ name: "t" }, "ctrl+t")).toBe(false)
    expect(matchesChord({ name: "t", ctrl: true, shift: true }, "ctrl+t")).toBe(false)
    expect(matchesChord({ name: "f10" }, "f10")).toBe(true)
  })
})

describe("loadKeybindings", () => {
  test("uses defaults without a file", () => {
    const loaded = loadKeybindings({}, () => "", () => false)
    expect(loaded.bindings).toEqual(DEFAULT_KEYBINDINGS)
    expect(loaded.errors).toEqual([])
  })

  test("overrides known actions and disables with null", () => {
    const loaded = loadKeybindings(
      { TRSS_KEYBINDINGS: "/tmp/keys.json" },
      () => JSON.stringify({ palette: "ctrl+space", frp: null, quit: "ctrl+q" }),
      () => true,
    )
    expect(loaded.bindings.palette).toBe("ctrl+space")
    expect(loaded.bindings.frp).toBe("")
    expect(loaded.bindings.quit).toBe("ctrl+q")
    expect(loaded.errors).toEqual([])
  })

  test("reports invalid and duplicate entries", () => {
    const loaded = loadKeybindings(
      {},
      () => JSON.stringify({ unknown: "ctrl+u", palette: "not a chord", frp: "ctrl+t" }),
      () => true,
    )
    expect(loaded.errors.length).toBeGreaterThanOrEqual(2)
    expect(loaded.bindings.frp).toBe(DEFAULT_KEYBINDINGS.frp)
  })

  test("survives malformed JSON", () => {
    const loaded = loadKeybindings({}, () => "{", () => true)
    expect(loaded.bindings).toEqual(DEFAULT_KEYBINDINGS)
    expect(loaded.errors.length).toBe(1)
  })
})

describe("matchesAction", () => {
  test("exposes metadata shortcuts", () => {
    expect(matchesAction({ name: "f2" }, DEFAULT_KEYBINDINGS, "favorite")).toBe(true)
    expect(matchesAction({ name: "f3" }, DEFAULT_KEYBINDINGS, "color")).toBe(true)
    expect(matchesAction({ name: "f4" }, DEFAULT_KEYBINDINGS, "labels")).toBe(true)
  })

  test("matches a configured action", () => {
    const bindings = { ...DEFAULT_KEYBINDINGS, hosts: "ctrl+h" }
    expect(matchesAction({ name: "h", ctrl: true }, bindings, "hosts")).toBe(true)
    expect(matchesAction({ name: "t", ctrl: true }, bindings, "hosts")).toBe(false)
  })
})
