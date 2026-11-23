import { describe, expect, test } from "bun:test"
import { isValidAlias, normalizeAlias } from "./alias.ts"

describe("host alias", () => {
  test("spaces become hyphens", () => {
    expect(normalizeAlias("vuigram server")).toBe("vuigram-server")
  })
  test("trims and strips unsafe characters", () => {
    expect(normalizeAlias("  mi host! ")).toBe("mi-host")
  })
  test("validates the ssh_config pattern", () => {
    expect(isValidAlias("vuigram-server")).toBe(true)
    expect(isValidAlias("vuigram server")).toBe(false)
    expect(isValidAlias("-bad")).toBe(false)
  })
})
