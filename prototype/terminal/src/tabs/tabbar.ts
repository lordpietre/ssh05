import { StyledText, bold, fg, type TextChunk } from "@opentui/core"
import type { SessionPhase, TabId, TabState } from "../../../../contracts/ts/index.ts"
import { PRODUCT, VENDOR } from "../identity.ts"
import { colorLevel, degradeColor } from "../theme/color.ts"

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

function phaseColor(phase: SessionPhase): string {
  switch (phase) {
    case "connected":
      return "#7BD88F"
    case "connecting":
    case "resolving":
    case "authenticating":
    case "reconnecting":
    case "degraded":
      return "#F2C94C"
    case "failed":
      return "#EB5757"
    case "closed":
    case "disconnected":
    case "detached":
      return "#8A94A6"
    default:
      return "#C9D1D9"
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

interface TabBarWindow {
  prefix: string
  suffix: string
  labels: string[]
  start: number
  end: number
  leftHidden: string
  rightHidden: string
  text: string
}

/**
 * Renders a fixed-width tab bar. The active tab is always visible; hidden tabs
 * are represented by ‹N left and N› right counters. The window maximizes the
 * number of visible tabs that fit in `width`.
 */
export function renderTabBar(options: TabBarOptions): string {
  const window = computeWindow(options)
  return window ? window.text : fit(` ${PRODUCT} · ${VENDOR}   Ctrl+T hosts   F10 salir`, Math.max(20, options.width))
}

/**
 * Colored variant. Tab color wins over the phase fallback. Colors degrade to
 * the terminal palette (truecolor → 256 → 16) and `NO_COLOR`/`SSH05_THEME=mono`
 * yield a plain, monochrome string.
 */
export function renderTabBarStyled(options: TabBarOptions): StyledText | string {  const level = colorLevel()
  if (level === "none") return renderTabBar(options)
  const window = computeWindow(options)
  if (!window) return renderTabBar(options)

  const chunks: TextChunk[] = [{ __isChunk: true, text: window.prefix }]
  if (window.leftHidden) chunks.push({ __isChunk: true, text: window.leftHidden })
  for (let index = window.start; index <= window.end; index++) {
    if (index > window.start) chunks.push({ __isChunk: true, text: " " })
    const tab = options.tabs[index]
    const color = degradeColor(tab.color || phaseColor(tab.phase), level)
    const label = window.labels[index]
    if (!color) {
      chunks.push({ __isChunk: true, text: label })
    } else {
      chunks.push(tab.id === options.activeId ? bold(fg(color)(label)) : fg(color)(label))
    }
  }
  if (window.rightHidden) chunks.push({ __isChunk: true, text: window.rightHidden })
  chunks.push({ __isChunk: true, text: window.suffix })
  return new StyledText(chunks)
}

function computeWindow(options: TabBarOptions): TabBarWindow | undefined {
  const width = Math.max(20, options.width)
  const prefix = options.prefix ?? ` ${PRODUCT} · `
  const suffix = options.suffix ?? " [+]"
  const tabs = options.tabs
  if (tabs.length === 0) return undefined

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

  if (bestText !== "") {
    return {
      prefix,
      suffix,
      labels,
      start: bestStart,
      end: bestEnd,
      leftHidden: bestStart > 0 ? `‹${bestStart} ` : "",
      rightHidden: bestEnd < tabs.length - 1 ? ` ${tabs.length - 1 - bestEnd}›` : "",
      text: bestText,
    }
  }

  // Even the active tab plus prefix/suffix does not fit: truncate it.
  const budget = Math.max(4, width - widthOf(prefix) - widthOf(suffix) - 2)
  const active = tabs[activeIndex]
  const label = `[${activeIndex + 1} ${phaseIcon(active.phase)} ${truncateTabTitle(active.title, budget - 4)}]`
  labels[activeIndex] = label
  return {
    prefix,
    suffix,
    labels,
    start: activeIndex,
    end: activeIndex,
    leftHidden: "",
    rightHidden: "",
    text: fit(`${prefix}${label}${suffix}`, width),
  }
}

function fit(value: string, width: number): string {
  if (widthOf(value) <= width) return value
  return Array.from(value).slice(0, width).join("")
}

export type TabBarHit =
  | { kind: "tab"; id: TabId }
  | { kind: "hidden-left" }
  | { kind: "hidden-right" }

/**
 * Maps a mouse column in the tab bar to a tab or to the hidden-tab counters.
 * It uses the same window layout as renderTabBarStyled.
 */
export function tabBarHitTest(options: TabBarOptions, x: number): TabBarHit | undefined {
  const window = computeWindow(options)
  if (!window) return undefined
  let cursor = widthOf(window.prefix)
  const leftWidth = widthOf(window.leftHidden)
  if (leftWidth > 0) {
    if (x >= cursor && x < cursor + leftWidth) return { kind: "hidden-left" }
    cursor += leftWidth
  }
  for (let index = window.start; index <= window.end; index++) {
    if (index > window.start) cursor += 1
    const label = window.labels[index] ?? ""
    const labelWidth = widthOf(label)
    if (x >= cursor && x < cursor + labelWidth) {
      const tab = options.tabs[index]
      return tab ? { kind: "tab", id: tab.id } : undefined
    }
    cursor += labelWidth
  }
  const rightWidth = widthOf(window.rightHidden)
  if (rightWidth > 0 && x >= cursor && x < cursor + rightWidth) return { kind: "hidden-right" }
  return undefined
}
