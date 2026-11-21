import { describe, expect, test } from "bun:test"
import { chmodSync, mkdtempSync, readFileSync, rmSync, statSync, writeFileSync } from "node:fs"
import { tmpdir } from "node:os"
import { join } from "node:path"
import type { Workspace } from "../../../../contracts/ts/index.ts"
import { listWorkspaceNames, loadWorkspaceFile, saveWorkspaceFile } from "./store.ts"

function workspace(): Workspace {
  return {
    id: "default",
    name: "default",
    tabs: [
      {
        id: "tab-1",
        title: "lab",
        hostProfileId: "lab",
        order: 0,
        tmux: { hostProfileId: "lab", sessionName: "trss-lab", verified: true },
      },
      { id: "tab-2", title: "vps", hostProfileId: "vps", order: 1 },
    ],
    preferences: { activeTabId: "tab-2" },
  }
}

function tempPath(): { dir: string; path: string } {
  const dir = mkdtempSync(join(tmpdir(), "trss-workspace-"))
  return { dir, path: join(dir, "nested", "workspace.json") }
}

describe("workspace store", () => {
  test("round-trips non-secret tab state", () => {
    const { dir, path } = tempPath()
    try {
      saveWorkspaceFile(workspace(), path)
      expect(loadWorkspaceFile(path)).toEqual(workspace())
    } finally {
      rmSync(dir, { recursive: true, force: true })
    }
  })

  test("never writes credentials or terminal bytes", () => {
    const { dir, path } = tempPath()
    try {
      saveWorkspaceFile(workspace(), path)
      const text = readFileSync(path, "utf8")
      for (const forbidden of ["password", "token", "secret", "ptyBytes"]) {
        expect(text.toLowerCase()).not.toContain(forbidden)
      }
    } finally {
      rmSync(dir, { recursive: true, force: true })
    }
  })

  test("uses private permissions on POSIX", () => {
    if (process.platform === "win32") return
    const { dir, path } = tempPath()
    try {
      saveWorkspaceFile(workspace(), path)
      expect(statSync(join(dir, "nested")).mode & 0o777).toBe(0o700)
      expect(statSync(path).mode & 0o777).toBe(0o600)
      chmodSync(path, 0o644)
      saveWorkspaceFile(workspace(), path)
      expect(statSync(path).mode & 0o777).toBe(0o600)
    } finally {
      rmSync(dir, { recursive: true, force: true })
    }
  })

  test("ignores malformed files", () => {
    const { dir, path } = tempPath()
    try {
      saveWorkspaceFile(workspace(), path)
      writeFileSync(path, "{not-json")
      expect(loadWorkspaceFile(path)).toBeUndefined()
    } finally {
      rmSync(dir, { recursive: true, force: true })
    }
  })
})

describe("workspace names", () => {
  test("lists files and always includes default", () => {
    const dir = mkdtempSync(join(tmpdir(), "trss-workspace-list-"))
    try {
      expect(listWorkspaceNames(dir)).toEqual(["default"])
      saveWorkspaceFile(workspace(), join(dir, "lab.json"))
      saveWorkspaceFile(workspace(), join(dir, "default.json"))
      expect(listWorkspaceNames(dir)).toEqual(["default", "lab"])
    } finally {
      rmSync(dir, { recursive: true, force: true })
    }
  })
})
