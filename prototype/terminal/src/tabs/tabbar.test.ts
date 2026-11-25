import { describe, expect, test } from "bun:test"
import type { TabState } from "../../../../contracts/ts/index.ts"
import { renderTabBar, renderTabBarStyled, tabBarHitTest } from "./tabbar.ts"
import { StyledText } from "@opentui/core"
import { resetColorLevel } from "../theme/color.ts"

function tabs(count: number): TabState[] {
  return Array.from({ length: count }, (_, index) => ({
    id: `tab-${index + 1}`,
    sessionId: `ses-${index + 1}`,
    title: `host-${index + 1}`,
    phase: "connected",
  }))
}

describe("renderTabBar", () => {
  test("shows all tabs when they fit", () => {
    const bar = renderTabBar({ tabs: tabs(3), activeId: "tab-2", width: 120 })
    expect(bar).toContain("[2 ● host-2]")
    expect(bar).not.toContain("‹")
    expect(bar).not.toContain("›")
    expect(bar.length).toBeLessThanOrEqual(120)
  })

  test("keeps the active tab visible with overflow counters", () => {
    const bar = renderTabBar({ tabs: tabs(10), activeId: "tab-6", width: 80 })
    expect(bar).toContain("[6 ● host-6]")
    expect(bar).toContain("‹")
    expect(bar).toContain("›")
    expect(Array.from(bar).length).toBeLessThanOrEqual(80)
  })

  test("active last tab still reserves the left counter", () => {
    const bar = renderTabBar({ tabs: tabs(10), activeId: "tab-10", width: 72 })
    expect(bar).toContain("[10 ● host-10]")
    expect(bar).toContain("‹")
    expect(Array.from(bar).length).toBeLessThanOrEqual(72)
  })

  test("truncates long titles", () => {
    const list = tabs(1)
    list[0].title = "a".repeat(100)
    const bar = renderTabBar({ tabs: list, activeId: "tab-1", width: 60 })
    expect(bar).toContain("…")
    expect(Array.from(bar).length).toBeLessThanOrEqual(60)
  })

  test("handles an empty workspace", () => {
    expect(renderTabBar({ tabs: [], width: 80 })).toContain("Ctrl+T hosts")
  })
})

describe("renderTabBarStyled", () => {
  test("preserves the computed text and colors tabs", () => {
    process.env.SSH05_COLOR_LEVEL = "truecolor"
    resetColorLevel()
    try {
      const options = { tabs: tabs(3), activeId: "tab-2", width: 120 }
      const styled = renderTabBarStyled(options)
      expect(styled).toBeInstanceOf(StyledText)
      if (!(styled instanceof StyledText)) return
      expect(styled.chunks.map((chunk) => chunk.text).join("")).toBe(renderTabBar(options))
      expect(styled.chunks.filter((chunk) => chunk.fg).length).toBe(3)
    } finally {
      delete process.env.SSH05_COLOR_LEVEL
      resetColorLevel()
    }
  })

  test("prefers the tab color over the phase color", () => {
    process.env.SSH05_COLOR_LEVEL = "truecolor"
    resetColorLevel()
    try {
      const list = tabs(1)
      list[0].color = "#112233"
      const styled = renderTabBarStyled({ tabs: list, activeId: undefined, width: 80 })
      if (!(styled instanceof StyledText)) throw new Error("expected styled output")
      const colored = styled.chunks.find((chunk) => chunk.fg)
      expect(colored?.fg?.toInts().slice(0, 3)).toEqual([0x11, 0x22, 0x33])
    } finally {
      delete process.env.SSH05_COLOR_LEVEL
      resetColorLevel()
    }
  })

  test("honors NO_COLOR", () => {
    const previous = process.env.NO_COLOR
    process.env.NO_COLOR = "1"
    resetColorLevel()
    try {
      expect(typeof renderTabBarStyled({ tabs: tabs(2), activeId: "tab-1", width: 80 })).toBe("string")
    } finally {
      if (previous === undefined) delete process.env.NO_COLOR
      else process.env.NO_COLOR = previous
      resetColorLevel()
    }
  })
})

describe("tabBarHitTest", () => {
  test("maps a click to the tab under the cursor", () => {
    const options = { tabs: tabs(3), activeId: "tab-2", width: 120 }
    const text = renderTabBar(options)
    const column = text.indexOf("[2 ● host-2]") + 2
    expect(tabBarHitTest(options, column)).toEqual({ kind: "tab", id: "tab-2" })
  })

  test("maps overflow counters", () => {
    const options = { tabs: tabs(10), activeId: "tab-6", width: 80 }
    const text = renderTabBar(options)
    expect(tabBarHitTest(options, text.indexOf("‹"))).toEqual({ kind: "hidden-left" })
    expect(tabBarHitTest(options, text.indexOf("›"))).toEqual({ kind: "hidden-right" })
  })

  test("returns undefined outside tabs and for an empty bar", () => {
    const options = { tabs: tabs(1), activeId: "tab-1", width: 80 }
    expect(tabBarHitTest(options, 0)).toBeUndefined()
    expect(tabBarHitTest({ tabs: [], width: 80 }, 10)).toBeUndefined()
  })

  test("hits a truncated active tab on very narrow terminals", () => {
    const list = tabs(1)
    list[0].title = "a".repeat(100)
    const options = { tabs: list, activeId: "tab-1", width: 24 }
    const text = renderTabBar(options)
    expect(tabBarHitTest(options, text.indexOf("["))).toEqual({ kind: "tab", id: "tab-1" })
  })
})
