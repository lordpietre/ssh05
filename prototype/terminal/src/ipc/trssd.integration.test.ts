import { test, expect } from "bun:test"
import { existsSync, mkdtempSync, rmSync } from "node:fs"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { IpcClient } from "./client.ts"

/**
 * Cross-language check: the TypeScript client against the real Go supervisor.
 *
 * TRSS_TEST_TRSSD=/path/to/trssd \
 * TRSS_TEST_SSH_CONFIG=/tmp/config \
 * TRSS_TEST_SSH_ALIAS=trss-test \
 * bun test src/ipc/trssd.integration.test.ts
 */
const trssdPath = process.env.TRSS_TEST_TRSSD
const sshConfig = process.env.TRSS_TEST_SSH_CONFIG
const alias = process.env.TRSS_TEST_SSH_ALIAS
const enabled = Boolean(trssdPath && sshConfig && alias)

test("TypeScript client talks to trssd end-to-end", async () => {
  if (!enabled) {
    console.log("SKIP: set TRSS_TEST_TRSSD, TRSS_TEST_SSH_CONFIG and TRSS_TEST_SSH_ALIAS")
    return
  }

  const dir = mkdtempSync(join(tmpdir(), "trss-ts-ipc-"))
  const socketPath = join(dir, "control.sock")
  const processHandle = Bun.spawn({
    cmd: [trssdPath!, "--socket", socketPath, "--ssh-config", sshConfig!],
    stderr: "pipe",
  })
  let client: IpcClient | undefined
  try {
    const deadline = Date.now() + 5000
    while (!existsSync(socketPath) && Date.now() < deadline) {
      await Bun.sleep(25)
    }
    if (!existsSync(socketPath)) {
      throw new Error("trssd did not create the socket")
    }

    client = await IpcClient.connect(socketPath)
    const hosts = await client.call("hosts.list", {}, { timeoutMs: 5000 })
    expect(hosts.hosts.some((host) => host.alias === alias)).toBe(true)

    const opened = await client.call("sessions.open", { hostProfileId: alias! }, { timeoutMs: 15000 })
    let transcript = ""
    let resolveOutput: (() => void) | undefined
    client.setBinaryHandler((_sessionId, data) => {
      transcript += new TextDecoder().decode(data)
      if (transcript.includes("30 100")) resolveOutput?.()
    })

    await client.call(
      "sessions.resize",
      { sessionId: opened.sessionId, cols: 100, rows: 30 },
      { timeoutMs: 5000 },
    )

    const outputReady = new Promise<void>((resolve) => {
      resolveOutput = resolve
    })
    client.sendBinary(opened.sessionId, new TextEncoder().encode("echo TRSS_TS_OK; stty size\n"))
    await Promise.race([
      outputReady,
      Bun.sleep(10000).then(() => {
        throw new Error(`timeout waiting for PTY output: ${transcript}`)
      }),
    ])

    await client.call(
      "sessions.close",
      { sessionId: opened.sessionId, intent: "close-view" },
      { timeoutMs: 5000 },
    )
  } finally {
    client?.close()
    processHandle.kill()
    rmSync(dir, { recursive: true, force: true })
  }
})
