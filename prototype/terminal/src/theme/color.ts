import { RGBA } from "@opentui/core"

/** Terminal color capabilities, from richest to none. */
export type ColorLevel = "truecolor" | "256" | "16" | "none"

/**
 * Detects the color capability of the hosting terminal. `TRSS_COLOR_LEVEL`
 * overrides detection (useful for tests and for a forced monochrome theme).
 * `NO_COLOR` and `TERM=dumb` disable color entirely.
 */
export function detectColorLevel(
  env: Record<string, string | undefined> = process.env,
  isTTY: boolean = process.stdout.isTTY ?? false,
): ColorLevel {
  const override = (env.TRSS_COLOR_LEVEL ?? "").toLowerCase()
  if (override === "truecolor" || override === "256" || override === "16" || override === "none") {
    return override
  }
  if (env.NO_COLOR !== undefined && env.NO_COLOR !== "") return "none"
  if ((env.TRSS_THEME ?? "").toLowerCase() === "mono") return "none"
  const term = (env.TERM ?? "").toLowerCase()
  if (term === "dumb") return "none"
  if (term === "") return isTTY ? "16" : "none"
  const colorTerm = (env.COLORTERM ?? "").toLowerCase()
  if (colorTerm === "truecolor" || colorTerm === "24bit") return "truecolor"
  if (term.includes("256color") || term.includes("256")) return "256"
  return "16"
}

/** Parses `#RGB`, `#RRGGBB` or `RRGGBB`; returns normalized `#RRGGBB`. */
export function normalizeHex(color: string | undefined): string | undefined {
  if (!color) return undefined
  const value = color.trim()
  if (!value) return undefined
  const short = /^#?([0-9a-fA-F]{3})$/.exec(value)
  if (short) {
    const [r, g, b] = Array.from(short[1])
    return `#${r}${r}${g}${g}${b}${b}`.toUpperCase()
  }
  const long = /^#?([0-9a-fA-F]{6})$/.exec(value)
  if (long) return `#${long[1].toUpperCase()}`
  return undefined
}

const ANSI16: readonly [number, number, number][] = [
  [0, 0, 0],
  [128, 0, 0],
  [0, 128, 0],
  [128, 128, 0],
  [0, 0, 128],
  [128, 0, 128],
  [0, 128, 128],
  [192, 192, 192],
  [128, 128, 128],
  [255, 0, 0],
  [0, 255, 0],
  [255, 255, 0],
  [0, 0, 255],
  [255, 0, 255],
  [0, 255, 255],
  [255, 255, 255],
]

function xterm256(index: number): [number, number, number] {
  if (index < 16) return ANSI16[index]
  if (index >= 232) {
    const level = 8 + (index - 232) * 10
    return [level, level, level]
  }
  const value = index - 16
  const steps = [0, 95, 135, 175, 215, 255]
  return [
    steps[Math.floor(value / 36) % 6],
    steps[Math.floor(value / 6) % 6],
    steps[value % 6],
  ]
}

function nearest(rgb: [number, number, number], count: number): number {
  let best = 0
  let bestDistance = Number.POSITIVE_INFINITY
  for (let index = 0; index < count; index++) {
    const [r, g, b] = xterm256(index)
    const distance = (r - rgb[0]) ** 2 + (g - rgb[1]) ** 2 + (b - rgb[2]) ** 2
    if (distance < bestDistance) {
      best = index
      bestDistance = distance
    }
  }
  return best
}

/**
 * Degrades a hex color for the detected capability. Returns an indexed RGBA
 * for 256/16-color terminals, a truecolor RGBA otherwise, or undefined when
 * color is disabled. Unknown values are passed through untouched.
 */
export function degradeColor(color: string | undefined, level: ColorLevel): RGBA | undefined {
  if (!color || level === "none") return undefined
  const hex = normalizeHex(color)
  if (!hex) return undefined
  const rgba = RGBA.fromHex(hex)
  if (level === "truecolor") return rgba
  const [r, g, b] = rgba.toInts()
  return RGBA.fromIndex(nearest([r, g, b], level === "16" ? 16 : 256))
}

/** Cached level so rendering does not re-read the environment on each frame. */
let cachedLevel: ColorLevel | undefined

export function colorLevel(): ColorLevel {
  if (cachedLevel === undefined) cachedLevel = detectColorLevel()
  return cachedLevel
}

/** Test helper: clears the cached capability. */
export function resetColorLevel(): void {
  cachedLevel = undefined
}
