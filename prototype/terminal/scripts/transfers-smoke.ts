/**
 * DAT-01 check: real SCP and SFTP upload/download through ssh05d.
 * Used by ui-smoke with the local sshd fixture.
 */
import { readFileSync, unlinkSync, writeFileSync } from "node:fs"
import { IpcClient } from "../src/ipc/client.ts"

const socket = process.env.SSH05_SOCKET
const alias = process.env.SSH05_TEST_SSH_ALIAS
if (!socket || !alias) {
  console.error("SSH05_SOCKET and SSH05_TEST_SSH_ALIAS are required")
  process.exit(2)
}

const hostAlias = alias as string
const stamp = `${Date.now()}-${process.pid}`
const localUpload = `/tmp/ssh05-xfer-upload-${stamp}.txt`
const localDownload = `/tmp/ssh05-xfer-download-${stamp}.txt`
const remote = `/tmp/ssh05-xfer-remote-${stamp}.txt`
const payload = "SSH05_TRANSFER_OK"
writeFileSync(localUpload, payload, { mode: 0o600 })

const client = await IpcClient.connect(socket)

async function waitTransfer(id: string, want: string): Promise<void> {
  const deadline = Date.now() + 15_000
  while (Date.now() < deadline) {
    const list = await client.call("transfers.list", {}, { timeoutMs: 5000 })
    const item = list.transfers.find((entry) => entry.transferId === id)
    if (item?.state === want) return
    if (item?.state === "failed") throw new Error(`transfer failed: ${item.errorLabel}`)
    await Bun.sleep(100)
  }
  throw new Error(`transfer ${id} did not reach ${want}`)
}

async function run(protocol: "scp" | "sftp"): Promise<void> {
  const uploaded = await client.call(
    "transfers.start",
    { hostProfileId: hostAlias, protocol, direction: "upload", localPath: localUpload, remotePath: remote },
    { timeoutMs: 15_000 },
  )
  await waitTransfer(uploaded.transferId, "completed")

  const downloaded = await client.call(
    "transfers.start",
    { hostProfileId: hostAlias, protocol, direction: "download", localPath: localDownload, remotePath: remote },
    { timeoutMs: 15_000 },
  )
  await waitTransfer(downloaded.transferId, "completed")
  if (readFileSync(localDownload, "utf8") !== payload) {
    throw new Error(`${protocol} download content mismatch`)
  }
}

try {
  await run("scp")
  await run("sftp")
  console.log("PASS: SCP and SFTP upload/download")
} finally {
  for (const path of [localUpload, localDownload]) {
    try {
      unlinkSync(path)
    } catch {
      // already removed
    }
  }
  try {
    unlinkSync(remote)
  } catch {
    // remote is localhost in this fixture
  }
  try {
    client.close()
  } catch {
    // already closed
  }
}
