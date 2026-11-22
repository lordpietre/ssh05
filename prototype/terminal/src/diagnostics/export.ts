import { chmodSync, mkdirSync, writeFileSync } from "node:fs"
import { join } from "node:path"
import type { DiagnosticsReport } from "../../../../contracts/ts/index.ts"
import { stateDir } from "../identity.ts"

/** State directory for sanitized diagnostics exports. */
export function diagnosticsDir(env: Record<string, string | undefined> = process.env): string {
  return join(stateDir(env), "diagnostics")
}

/** Writes the report with private permissions and returns its path. */
export function exportDiagnostics(
  report: DiagnosticsReport,
  dir: string = diagnosticsDir(),
  now: Date = new Date(),
): string {
  mkdirSync(dir, { recursive: true, mode: 0o700 })
  chmodSync(dir, 0o700)
  const stamp = now.toISOString().replace(/[:.]/g, "-")
  const path = join(dir, `ssh05-diagnostics-${stamp}.json`)
  writeFileSync(path, `${JSON.stringify(report, null, 2)}\n`, { encoding: "utf8", mode: 0o600 })
  chmodSync(path, 0o600)
  return path
}
