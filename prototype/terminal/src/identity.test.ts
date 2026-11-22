import { describe, expect, test } from "bun:test"
import { mkdtempSync, mkdirSync, writeFileSync, existsSync } from "node:fs"
import { tmpdir } from "node:os"
import { join } from "node:path"
import {
  configDir,
  envValue,
  LEGACY_PRODUCT,
  PRODUCT,
  resolveSocketPath,
  SUPPORT_EMAIL,
  VENDOR,
} from "./identity.ts"

describe("identity", () => {
  test("publishes vendor contact", () => {
    expect(VENDOR).toBe("Dinámicas Electrónicas")
    expect(SUPPORT_EMAIL).toBe("dinamicas.electronicas@gmail.com")
  })

  test("envValue prefers SSH05 and falls back to TRSS", () => {
    expect(envValue("SOCKET", { SSH05_SOCKET: "/new", TRSS_SOCKET: "/old" })).toBe("/new")
    expect(envValue("SOCKET", { TRSS_SOCKET: "/old" })).toBe("/old")
    expect(envValue("SOCKET", {})).toBeUndefined()
  })

  test("configDir adopts the legacy trss directory", () => {
    const base = mkdtempSync(join(tmpdir(), "ssh05-identity-"))
    const legacy = join(base, LEGACY_PRODUCT)
    mkdirSync(legacy)
    writeFileSync(join(legacy, "hosts.json"), "{}")
    expect(configDir({ XDG_CONFIG_HOME: base })).toBe(join(base, PRODUCT))
    expect(existsSync(join(base, PRODUCT, "hosts.json"))).toBe(true)
  })

  test("resolveSocketPath accepts an explicit override", () => {
    expect(resolveSocketPath({ SSH05_SOCKET: "/tmp/new.sock" })).toBe("/tmp/new.sock")
    expect(resolveSocketPath({ TRSS_SOCKET: "/tmp/old.sock" })).toBe("/tmp/old.sock")
  })
})
