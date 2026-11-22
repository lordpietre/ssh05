import { chmodSync, mkdirSync, readFileSync, writeFileSync } from "node:fs"
import { dirname, join } from "node:path"
import { configDir, envValue } from "../identity.ts"

export type Density = "compact" | "comfortable"
export type TabPosition = "top" | "bottom"

export interface Preferences {
  hostPanelWidth: number
  density: Density
  tabPosition: TabPosition
}

export const DEFAULT_PREFERENCES: Preferences = {
  hostPanelWidth: 30,
  density: "compact",
  tabPosition: "top",
}

export const PANEL_WIDTHS = [24, 30, 36, 42, 48]
export const MIN_PANEL_WIDTH = 20
export const MAX_PANEL_WIDTH = 60

export function defaultPreferencesPath(env: Record<string, string | undefined> = process.env): string {
  return envValue("PREFERENCES", env) ?? join(configDir(env), "preferences.json")
}

export function sanitizePreferences(input: unknown): Preferences {
  const preferences = { ...DEFAULT_PREFERENCES }
  if (!input || typeof input !== "object" || Array.isArray(input)) return preferences
  const data = input as Record<string, unknown>
  if (typeof data.hostPanelWidth === "number" && Number.isFinite(data.hostPanelWidth)) {
    preferences.hostPanelWidth = Math.min(
      MAX_PANEL_WIDTH,
      Math.max(MIN_PANEL_WIDTH, Math.round(data.hostPanelWidth)),
    )
  }
  if (data.density === "compact" || data.density === "comfortable") {
    preferences.density = data.density
  }
  if (data.tabPosition === "top" || data.tabPosition === "bottom") {
    preferences.tabPosition = data.tabPosition
  }
  return preferences
}

export function loadPreferences(
  path: string = defaultPreferencesPath(),
  read: (path: string) => string = (file) => readFileSync(file, "utf8"),
): Preferences {
  try {
    return sanitizePreferences(JSON.parse(read(path)))
  } catch {
    return { ...DEFAULT_PREFERENCES }
  }
}

export function savePreferences(
  preferences: Preferences,
  path: string = defaultPreferencesPath(),
): void {
  const dir = dirname(path)
  mkdirSync(dir, { recursive: true, mode: 0o700 })
  chmodSync(dir, 0o700)
  writeFileSync(path, `${JSON.stringify(preferences, null, 2)}\n`, {
    encoding: "utf8",
    mode: 0o600,
  })
  chmodSync(path, 0o600)
}

export function nextPanelWidth(current: number, direction = 1): number {
  const index = PANEL_WIDTHS.indexOf(current)
  const next = (index < 0 ? 1 : index + direction + PANEL_WIDTHS.length) % PANEL_WIDTHS.length
  return PANEL_WIDTHS[next]
}
