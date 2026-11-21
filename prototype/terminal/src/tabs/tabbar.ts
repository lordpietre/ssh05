import type { SessionPhase, TabId, TabState } from "../../../../contracts/ts/index.ts"

export function phaseIcon(phase: SessionPhase): string {
  switch (phase) {
    case "connecting":
    case "resolving":
    case "authenticating":
      return "↻"
    case "connected":
      return "●"
    case "degraded":
    case "reconnecting":
      return "!"
    case "disconnected":
    case "detached":
      return "○"
    case "failed":
      return "!"
    case "closing":
    case "closed":
      return "×"
    default:
      return "·"
  }
}

export function phaseLabel(phase: SessionPhase): string {
  switch (phase) {
    case "connecting":
      return "conectando"
    case "resolving":
      return "resolviendo"
    case "authenticating":
      return "autenticando"
    case "connected":
      return "conectado"
    case "degraded":
      return "degradado"
    case "reconnecting":
      return "reconectando"
    case "disconnected":
      return "desconectado"
    case "detached":
      return "desacoplado"
    case "failed":
      return "error"
    case "closing":
      return "cerrando"
    case "closed":
      return "cerrado"
    default:
      return phase
  }
}

function widthOf(value: string): number {
  return Array.from(value).length
}

export function truncateTabTitle(value: string, max: number): string {
  if (widthOf(value) <= max) return value
  const chars = Array.from(value)
  return `${chars.slice(0, Math.max(0, max - 1)).join("")}…`
}

export interface TabBarOptions {
  tabs: readonly TabState[]
  activeId?: TabId
  width: number
  prefix?: string
  suffix?: string
}

/**
 * Renders a fixed-width tab bar. The active tab is always visible; hidden tabs
 * are represented by ‹N left and N› right counters. The window maximizes the
 * number of visible tabs that fit in `width`.
 */
export function renderTabBar(options: TabBarOptions): string {
  const width = Math.max(20, options.width)
  const prefix = options.prefix ?? " TRSS · "
  const suffix = options.suffix ?? " [+]"
  const tabs = options.tabs
  if (tabs.length === 0) {
    return fit(" TRSS · supervisor   Ctrl+T hosts   F10 salir", width)
  }

  const labels = tabs.map((tab, index) => {
    const label = `${index + 1} ${phaseIcon(tab.phase)} ${truncateTabTitle(tab.title, 18)}`
    return tab.id === options.activeId ? `[${label}]` : ` ${label} `
  })

  const activeIndex = Math.max(
    0,
    tabs.findIndex((tab) => tab.id === options.activeId),
  )

  let bestStart = activeIndex
  let bestEnd = activeIndex
  let bestText = ""

  for (let start = 0; start <= activeIndex; start++) {
    for (let end = activeIndex; end < tabs.length; end++) {
      const leftHidden = start > 0 ? `‹${start} ` : ""
      const rightHidden = end < tabs.length - 1 ? ` ${tabs.length - 1 - end}›` : ""
      const text = `${prefix}${leftHidden}${labels.slice(start, end + 1).join(" ")}${rightHidden}${suffix}`
      if (widthOf(text) <= width && end - start > bestEnd - bestStart) {
        bestStart = start
        bestEnd = end
        bestText = text
      }
    }
  }

  if (bestText !== "") return bestText

  // Even the active tab plus prefix/suffix does not fit: truncate it.
  const budget = Math.max(4, width - widthOf(prefix) - widthOf(suffix) - 2)
  const active = tabs[activeIndex]
  const label = `[${activeIndex + 1} ${phaseIcon(active.phase)} ${truncateTabTitle(active.title, budget - 4)}]`
  return fit(`${prefix}${label}${suffix}`, width)
}

function fit(value: string, width: number): string {
  if (widthOf(value) <= width) return value
  return Array.from(value).slice(0, width).join("")
}
