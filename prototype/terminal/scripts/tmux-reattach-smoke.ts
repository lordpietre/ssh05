/**
 * SES-08 check: restoring a tab must only attach an existing tmux session.
 * Used by ui-smoke with a real sshd and a running trssd.
 */
import { IpcClient } from "../src/ipc/client.ts"

const socket = process.env.TRSS_SOCKET
const alias = process.env.TRSS_TEST_SSH_ALIAS
if (!socket || !alias) {
  console.error("TRSS_SOCKET and TRSS_TEST_SSH_ALIAS are required")
  process.exit(2)
}

const client = await IpcClient.connect(socket)
const missingName = `trss-missing-${Date.now()}`
const createdName = `trss-created-${Date.now()}`
let createdID = ""

try {
  const missing = await client.call(
    "sessions.open",
    { hostProfileId: alias, tmuxSession: missingName, tmuxCreate: false },
    { timeoutMs: 20_000 },
  )
  if (missing.tmuxSession) {
    throw new Error(`missing tmux was recreated: ${missing.tmuxSession}`)
  }
  await client.call("sessions.close", { sessionId: missing.sessionId, intent: "close-view" }, { timeoutMs: 5000 })

  const created = await client.call(
    "sessions.open",
    { hostProfileId: alias, tmuxSession: createdName, tmuxCreate: true },
    { timeoutMs: 20_000 },
  )
  createdID = created.sessionId
  if (created.tmuxSession !== createdName) {
    throw new Error(`created tmux not reported: ${JSON.stringify(created)}`)
  }
  const listed = await client.call("sessions.list", {}, { timeoutMs: 5000 })
  const info = listed.sessions.find((item) => item.sessionId === createdID)
  if (!info || info.tmuxSession !== createdName) {
    throw new Error(`sessions.list lost tmux binding: ${JSON.stringify(info)}`)
  }
  console.log("PASS: missing tmux falls back and created tmux is reported")
} finally {
  if (createdID) {
    await client
      .call("sessions.close", { sessionId: createdID, intent: "close-view" }, { timeoutMs: 5000 })
      .catch(() => undefined)
  }
  try {
    client.close()
  } catch {
    // already closed
  }
}
