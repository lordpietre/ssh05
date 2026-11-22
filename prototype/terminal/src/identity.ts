import { existsSync, renameSync } from "node:fs"
import { homedir, tmpdir } from "node:os"
import { join } from "node:path"

/** Product and vendor identity shared with the Go supervisor. */
export const PRODUCT = "ssh05"
export const SUPERVISOR = "ssh05d"
export const LEGACY_PRODUCT = "trss"
export const VENDOR = "Dinámicas Electrónicas"
export const SUPPORT_EMAIL = "dinamicas.electronicas@gmail.com"

export type Env = Record<string, string | undefined>

/**
 * Reads SSH05_<suffix>, falling back to the legacy TRSS_<suffix> name so old
 * scripts and services keep working during the rename.
 */
export function envValue(suffix: string, env: Env = process.env): string | undefined {
  const current = env[`SSH05_${suffix}`]?.trim()
  if (current) return current
  return env[`TRSS_${suffix}`]?.trim() || undefined
}

function xdgBase(env: Env, key: string, fallback: string): string {
  return env[key]?.trim() || join(env.HOME || homedir(), fallback)
}

/**
 * Returns ~/.config/ssh05, adopting ~/.config/trss once when it is the only
 * directory that exists. Writes always target the returned directory.
 */
export function configDir(env: Env = process.env): string {
  return adoptLegacyDir(xdgBase(env, "XDG_CONFIG_HOME", ".config"))
}

/** Returns ~/.local/state/ssh05 with the same one-time adoption. */
export function stateDir(env: Env = process.env): string {
  return adoptLegacyDir(xdgBase(env, "XDG_STATE_HOME", join(".local", "state")))
}

function adoptLegacyDir(base: string): string {
  const target = join(base, PRODUCT)
  const legacy = join(base, LEGACY_PRODUCT)
  if (existsSync(target)) return target
  if (existsSync(legacy)) {
    try {
      renameSync(legacy, target)
    } catch {
      // A read-only or cross-device home keeps using the legacy directory.
      return legacy
    }
  }
  return target
}

function uid(): string {
  return typeof process.getuid === "function" ? String(process.getuid()) : "user"
}

/** Default control socket path for the current platform. */
export function socketPath(env: Env = process.env): string {
  const override = envValue("SOCKET", env)
  if (override) return override
  const runtimeDir = env.XDG_RUNTIME_DIR?.trim()
  if (runtimeDir) return join(runtimeDir, PRODUCT, "control.sock")
  return join(tmpdir(), `${PRODUCT}-${uid()}`, "control.sock")
}

/** Socket path used before the rename. */
export function legacySocketPath(env: Env = process.env): string {
  const runtimeDir = env.XDG_RUNTIME_DIR?.trim()
  if (runtimeDir) return join(runtimeDir, LEGACY_PRODUCT, "control.sock")
  return join(tmpdir(), `${LEGACY_PRODUCT}-${uid()}`, "control.sock")
}

/**
 * Resolves the socket to connect to: an explicit SSH05_SOCKET/TRSS_SOCKET wins,
 * then the current path, then a live legacy socket from an older supervisor.
 */
export function resolveSocketPath(env: Env = process.env): string {
  const explicit = envValue("SOCKET", env)
  if (explicit) return explicit
  const current = socketPath(env)
  if (existsSync(current)) return current
  const legacy = legacySocketPath(env)
  if (existsSync(legacy)) return legacy
  return current
}
