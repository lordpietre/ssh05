import {
  chmodSync,
  existsSync,
  mkdirSync,
  readdirSync,
  readFileSync,
  renameSync,
  writeFileSync,
} from "node:fs"
import { basename, dirname, join } from "node:path"
import type { Workspace } from "../../../../contracts/ts/index.ts"
import { configDir, envValue } from "../identity.ts"

/**
 * Workspace files contain only non-secret view state: titles, host profile
 * ids, session ids, order and optional tmux names. Never PTY bytes or secrets.
 */
export function workspaceDir(): string {
  return envValue("WORKSPACE_DIR") ?? join(configDir(), "workspaces")
}

export function safeWorkspaceName(name: string): string {
  const safe = name.trim().replace(/[^A-Za-z0-9_-]+/g, "-").replace(/^-+|-+$/g, "")
  return safe || "default"
}

export function workspacePath(name = "default"): string {
  const override = envValue("WORKSPACE")
  if (override) return override
  return join(workspaceDir(), `${safeWorkspaceName(name)}.json`)
}

export function listWorkspaceNames(dir = workspaceDir()): string[] {
  if (envValue("WORKSPACE")) return ["default"]
  const names = new Set<string>(["default"])
  try {
    for (const file of readdirSync(dir)) {
      if (!file.endsWith(".json")) continue
      const name = basename(file, ".json")
      if (name) names.add(name)
    }
  } catch {
    // Missing directory means only the default workspace exists.
  }
  return [...names].sort((a, b) => a.localeCompare(b))
}

export function loadWorkspaceFile(path: string): Workspace | undefined {
  if (!existsSync(path)) return undefined
  try {
    const parsed = JSON.parse(readFileSync(path, "utf8")) as Workspace
    if (!parsed || typeof parsed !== "object" || !Array.isArray(parsed.tabs)) return undefined
    return {
      id: typeof parsed.id === "string" ? parsed.id : "default",
      name: typeof parsed.name === "string" ? parsed.name : "default",
      tabs: parsed.tabs.filter(
        (tab) => tab && typeof tab.id === "string" && typeof tab.hostProfileId === "string",
      ),
      preferences: parsed.preferences ?? {},
    }
  } catch {
    return undefined
  }
}

export function loadWorkspace(name = "default"): Workspace | undefined {
  return loadWorkspaceFile(workspacePath(name))
}

export function saveWorkspaceFile(workspace: Workspace, path: string): void {
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

export function saveWorkspace(workspace: Workspace, name = "default"): void {
  saveWorkspaceFile(workspace, workspacePath(name))
}
