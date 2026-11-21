import {
  chmodSync,
  existsSync,
  mkdirSync,
  readFileSync,
  renameSync,
  writeFileSync,
} from "node:fs"
import { homedir } from "node:os"
import { dirname, join } from "node:path"
import type { Workspace } from "../../../../contracts/ts/index.ts"

/**
 * Workspace files contain only non-secret view state: titles, host profile
 * ids, order and optional tmux names. Never PTY bytes or credentials.
 */
export function workspacePath(): string {
  if (process.env.TRSS_WORKSPACE) return process.env.TRSS_WORKSPACE
  const configHome = process.env.XDG_CONFIG_HOME || join(homedir(), ".config")
  return join(configHome, "trss", "workspace.json")
}

export function loadWorkspace(path = workspacePath()): Workspace | undefined {
  if (!existsSync(path)) return undefined
  try {
    const parsed = JSON.parse(readFileSync(path, "utf8")) as Workspace
    if (!parsed || typeof parsed !== "object" || !Array.isArray(parsed.tabs)) return undefined
    return {
      id: typeof parsed.id === "string" ? parsed.id : "default",
      name: typeof parsed.name === "string" ? parsed.name : "default",
      tabs: parsed.tabs.filter((tab) => tab && typeof tab.id === "string" && typeof tab.hostProfileId === "string"),
      preferences: parsed.preferences ?? {},
    }
  } catch {
    return undefined
  }
}

export function saveWorkspace(workspace: Workspace, path = workspacePath()): void {
  const dir = dirname(path)
  mkdirSync(dir, { recursive: true, mode: 0o700 })
  try {
    chmodSync(dir, 0o700)
  } catch {
    // Best effort on platforms without POSIX modes.
  }
  const temporary = `${path}.tmp`
  writeFileSync(temporary, `${JSON.stringify(workspace, null, 2)}\n`, { mode: 0o600 })
  renameSync(temporary, path)
  try {
    chmodSync(path, 0o600)
  } catch {
    // Best effort.
  }
}
