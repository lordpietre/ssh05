import { describe, expect, test } from "bun:test"
import type { TabState, TabStoreEvent } from "../../../../contracts/ts/index.ts"
import { InMemoryTabStore } from "./store.ts"

function tab(id: string, title = id): TabState {
  return {
    id,
    sessionId: `session-${id}`,
    title,
    phase: "connected",
  }
}

describe("InMemoryTabStore", () => {
  test("adds tabs and activates the first one", () => {
    const store = new InMemoryTabStore()
    store.add(tab("a"))
    store.add(tab("b"))
    expect(store.list().map((item) => item.id)).toEqual(["a", "b"])
    expect(store.active()?.id).toBe("a")
  })

  test("activates, moves and updates tabs", () => {
    const store = new InMemoryTabStore()
    store.add(tab("a"))
    store.add(tab("b"))
    store.add(tab("c"))
    const events: TabStoreEvent[] = []
    store.subscribe((event) => events.push(event))

    store.activate("c")
    expect(store.active()?.id).toBe("c")
    store.move("c", 0)
    expect(store.list().map((item) => item.id)).toEqual(["c", "a", "b"])
    store.update("c", { title: "renamed", phase: "degraded" })
    expect(store.active()?.title).toBe("renamed")
    expect(store.active()?.phase).toBe("degraded")
    expect(events).toEqual([
      { type: "activated", tabId: "c" },
      { type: "moved", tabId: "c", index: 0 },
      { type: "updated", tabId: "c" },
    ])
  })

  test("removes the active tab and activates the next one", () => {
    const store = new InMemoryTabStore()
    store.add(tab("a"))
    store.add(tab("b"))
    store.add(tab("c"))
    store.activate("b")
    const events: TabStoreEvent[] = []
    store.subscribe((event) => events.push(event))

    store.remove("b")
    expect(store.list().map((item) => item.id)).toEqual(["a", "c"])
    expect(store.active()?.id).toBe("c")
    expect(events).toEqual([
      { type: "removed", tabId: "b" },
      { type: "activated", tabId: "c" },
    ])
  })

  test("removing the last tab leaves no active tab", () => {
    const store = new InMemoryTabStore()
    store.add(tab("a"))
    store.remove("a")
    expect(store.list()).toHaveLength(0)
    expect(store.active()).toBeUndefined()
  })

  test("unsubscribe stops events", () => {
    const store = new InMemoryTabStore()
    store.add(tab("a"))
    let count = 0
    const unsubscribe = store.subscribe(() => count++)
    unsubscribe()
    store.add(tab("b"))
    expect(count).toBe(0)
  })
})
