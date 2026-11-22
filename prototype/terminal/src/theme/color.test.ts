import { describe, expect, test } from "bun:test"
import { degradeColor, detectColorLevel, normalizeHex } from "./color.ts"

describe("detectColorLevel", () => {
  test("honors an explicit override", () => {
    expect(detectColorLevel({ SSH05_COLOR_LEVEL: "256" }, false)).toBe("256")
    expect(detectColorLevel({ SSH05_COLOR_LEVEL: "none" }, true)).toBe("none")
  })

  test("detects truecolor, 256 and 16", () => {
    expect(detectColorLevel({ TERM: "xterm-256color", COLORTERM: "truecolor" }, true)).toBe(
      "truecolor",
    )
    expect(detectColorLevel({ TERM: "xterm-256color" }, true)).toBe("256")
    expect(detectColorLevel({ TERM: "xterm" }, true)).toBe("16")
  })

  test("disables color for NO_COLOR, mono theme and dumb terminals", () => {
    expect(detectColorLevel({ NO_COLOR: "1", TERM: "xterm" }, true)).toBe("none")
    expect(detectColorLevel({ SSH05_THEME: "mono", TERM: "xterm" }, true)).toBe("none")
    expect(detectColorLevel({ TERM: "dumb" }, true)).toBe("none")
  })
})

describe("normalizeHex", () => {
  test("normalizes short and long forms", () => {
    expect(normalizeHex("#abc")).toBe("#AABBCC")
    expect(normalizeHex("5878cc")).toBe("#5878CC")
    expect(normalizeHex("not-a-color")).toBeUndefined()
    expect(normalizeHex("")).toBeUndefined()
  })
})

describe("degradeColor", () => {
  test("returns truecolor unchanged", () => {
    const color = degradeColor("#5878CC", "truecolor")
    expect(color?.toInts().slice(0, 3)).toEqual([0x58, 0x78, 0xcc])
  })

  test("returns indexed colors for 256 and 16", () => {
    const indexed = degradeColor("#5878CC", "256")
    expect(indexed?.intent).toBe("indexed")
    const ansi = degradeColor("#5878CC", "16")
    expect(ansi?.intent).toBe("indexed")
    expect(((ansi?.meta ?? 0) & 0xff)).toBeLessThan(16)
  })

  test("returns undefined without color", () => {
    expect(degradeColor("#5878CC", "none")).toBeUndefined()
    expect(degradeColor(undefined, "truecolor")).toBeUndefined()
    expect(degradeColor("bogus", "truecolor")).toBeUndefined()
  })
})
