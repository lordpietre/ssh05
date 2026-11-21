import { describe, expect, test } from "bun:test"
import type { TabState } from "../../../../contracts/ts/index.ts"
import { renderTabBar } from "./tabbar.ts"

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
