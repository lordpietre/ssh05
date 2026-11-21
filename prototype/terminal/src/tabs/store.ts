import type { TabId, TabState, TabStore, TabStoreEvent } from "../../../../contracts/ts/index.ts"

/**
 * Renderer-independent tab state. Closing a tab removes its view only; the
 * caller decides the backend close intent.
 */
export class InMemoryTabStore implements TabStore {
  private tabs: TabState[] = []
  private activeId: TabId | undefined
  private readonly listeners = new Set<(event: TabStoreEvent) => void>()

  list(): readonly TabState[] {
    return [...this.tabs]
  }

  active(): TabState | undefined {
    return this.tabs.find((tab) => tab.id === this.activeId)
  }

  add(tab: TabState, index?: number): void {
    if (this.tabs.some((item) => item.id === tab.id)) {
      throw new Error(`tab ${tab.id} already exists`)
    }
    const at = index === undefined ? this.tabs.length : clamp(index, 0, this.tabs.length)
    this.tabs.splice(at, 0, tab)
    this.emit({ type: "added", tabId: tab.id, index: at })
    if (this.activeId === undefined) {
      this.activeId = tab.id
      this.emit({ type: "activated", tabId: tab.id })
    }
  }

  remove(id: TabId): void {
    const index = this.indexOf(id)
    if (index < 0) return
    this.tabs.splice(index, 1)
    this.emit({ type: "removed", tabId: id })
    if (this.activeId !== id) return

    const next = this.tabs[Math.min(index, this.tabs.length - 1)]
    this.activeId = next?.id
    if (next) this.emit({ type: "activated", tabId: next.id })
  }

  activate(id: TabId): void {
    if (this.indexOf(id) < 0 || this.activeId === id) return
    this.activeId = id
    this.emit({ type: "activated", tabId: id })
  }

  move(id: TabId, index: number): void {
    const from = this.indexOf(id)
    if (from < 0) return
    const to = clamp(index, 0, this.tabs.length - 1)
    if (from === to) return
    const [tab] = this.tabs.splice(from, 1)
    this.tabs.splice(to, 0, tab)
    this.emit({ type: "moved", tabId: id, index: to })
  }

  update(id: TabId, patch: Partial<Omit<TabState, "id">>): void {
    const tab = this.tabs.find((item) => item.id === id)
    if (!tab) return
    Object.assign(tab, patch)
    this.emit({ type: "updated", tabId: id })
  }

  subscribe(listener: (event: TabStoreEvent) => void): () => void {
    this.listeners.add(listener)
    return () => this.listeners.delete(listener)
  }

  private indexOf(id: TabId): number {
    return this.tabs.findIndex((tab) => tab.id === id)
  }

  private emit(event: TabStoreEvent): void {
    for (const listener of this.listeners) listener(event)
  }
}

function clamp(value: number, min: number, max: number): number {
  return Math.max(min, Math.min(max, Math.trunc(value)))
}
