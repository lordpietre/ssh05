/** Normalizes a host alias typed in the UI to the ssh_config-safe form. */
export function normalizeAlias(value: string): string {
  return value
    .trim()
    .replace(/\s+/g, "-")
    .replace(/[^A-Za-z0-9._-]/g, "")
    .replace(/^[^A-Za-z0-9]+/, "")
    .slice(0, 64)
}

export function isValidAlias(value: string): boolean {
  return /^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$/.test(value)
}
