/**
 * DIA-01 check: DNS, TCP, banner and latency through trssd.
 * Used by ui-smoke with a real sshd.
 */
import { IpcClient } from "../src/ipc/client.ts"

const socket = process.env.TRSS_SOCKET
const alias = process.env.TRSS_TEST_SSH_ALIAS
if (!socket || !alias) {
  console.error("TRSS_SOCKET and TRSS_TEST_SSH_ALIAS are required")
  process.exit(2)
}

const client = await IpcClient.connect(socket)
try {
  const report = await client.call("diagnostics.host", { hostProfileId: alias }, { timeoutMs: 15_000 })
  if (report.tcp.state !== "reachable") {
    throw new Error(`TCP not reachable: ${JSON.stringify(report.tcp)}`)
  }
  if (!report.dns.addresses || report.dns.addresses.length === 0) {
    throw new Error(`DNS empty: ${JSON.stringify(report.dns)}`)
  }
  if (!report.banner.received || !report.banner.received.startsWith("SSH-")) {
    throw new Error(`unexpected banner: ${JSON.stringify(report.banner)}`)
  }
  console.log(
    `PASS: diagnostics ${report.address} dns=${report.dns.addresses.length} tcp=${report.tcp.latencyMs}ms banner=${report.banner.received}`,
  )
} finally {
  try {
    client.close()
  } catch {
    // already closed
  }
}
