import { describe, expect, test } from "bun:test"
import { mkdtempSync, readFileSync, statSync } from "node:fs"
import { tmpdir } from "node:os"
import { join } from "node:path"
import type { DiagnosticsReport } from "../../../../contracts/ts/index.ts"
import { diagnosticsDir, exportDiagnostics } from "./export.ts"

const report: DiagnosticsReport = {
  generatedAt: "2026-11-22T00:00:00Z",
  protocolVersion: 1,
  hostsCount: 2,
  conflicts: 0,
  sessions: [],
  vault: { provider: "none", locked: true, profiles: 0 },
  frp: { endpoints: [], proxies: [] },
}

describe("diagnosticsDir", () => {
  test("prefers XDG_STATE_HOME", () => {
    expect(diagnosticsDir({ XDG_STATE_HOME: "/tmp/state", HOME: "/home/x" })).toBe(
      "/tmp/state/trss/diagnostics",
    )
  })

  test("falls back to HOME", () => {
    expect(diagnosticsDir({ HOME: "/home/x" })).toBe("/home/x/.local/state/trss/diagnostics")
  })
})

describe("exportDiagnostics", () => {
  test("writes private JSON and returns its path", () => {
    const dir = mkdtempSync(join(tmpdir(), "trss-diag-"))
    const target = join(dir, "nested")
    const path = exportDiagnostics(report, target, new Date("2026-11-22T01:02:03.456Z"))
    expect(path.endsWith("trss-diagnostics-2026-11-22T01-02-03-456Z.json")).toBe(true)
    expect(JSON.parse(readFileSync(path, "utf8"))).toEqual(report)
    if (process.platform !== "win32") {
      expect(statSync(target).mode & 0o777).toBe(0o700)
      expect(statSync(path).mode & 0o777).toBe(0o600)
    }
  })
})
