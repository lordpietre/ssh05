import { describe, expect, test } from "bun:test"
import { mkdtempSync, readFileSync, statSync } from "node:fs"
import { tmpdir } from "node:os"
import { join } from "node:path"
import {
  DEFAULT_PREFERENCES,
  loadPreferences,
  nextPanelWidth,
  sanitizePreferences,
  savePreferences,
} from "./store.ts"

describe("sanitizePreferences", () => {
  test("uses defaults for invalid input", () => {
    expect(sanitizePreferences(null)).toEqual(DEFAULT_PREFERENCES)
    expect(sanitizePreferences([])).toEqual(DEFAULT_PREFERENCES)
    expect(sanitizePreferences({ density: "huge", tabPosition: "left" })).toEqual(
      DEFAULT_PREFERENCES,
    )
  })

  test("clamps width and accepts valid values", () => {
    expect(sanitizePreferences({ hostPanelWidth: 5 }).hostPanelWidth).toBe(20)
    expect(sanitizePreferences({ hostPanelWidth: 100 }).hostPanelWidth).toBe(60)
    expect(
      sanitizePreferences({ hostPanelWidth: 36, density: "comfortable", tabPosition: "bottom" }),
    ).toEqual({ hostPanelWidth: 36, density: "comfortable", tabPosition: "bottom" })
  })
})

describe("loadPreferences", () => {
  test("survives missing or malformed files", () => {
    expect(loadPreferences("/nope", () => "{")).toEqual(DEFAULT_PREFERENCES)
  })
})

describe("savePreferences", () => {
  test("writes private JSON", () => {
    const dir = mkdtempSync(join(tmpdir(), "trss-prefs-"))
    const path = join(dir, "nested", "preferences.json")
    savePreferences({ hostPanelWidth: 42, density: "compact", tabPosition: "bottom" }, path)
    expect(JSON.parse(readFileSync(path, "utf8"))).toEqual({
      hostPanelWidth: 42,
      density: "compact",
      tabPosition: "bottom",
    })
    if (process.platform !== "win32") {
      expect(statSync(path).mode & 0o777).toBe(0o600)
      expect(statSync(join(dir, "nested")).mode & 0o777).toBe(0o700)
    }
  })
})

describe("nextPanelWidth", () => {
  test("cycles through the presets", () => {
    expect(nextPanelWidth(30)).toBe(36)
    expect(nextPanelWidth(48)).toBe(24)
    expect(nextPanelWidth(24, -1)).toBe(48)
    expect(nextPanelWidth(31)).toBe(30)
  })
})
