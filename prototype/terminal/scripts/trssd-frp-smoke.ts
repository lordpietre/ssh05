/**
 * Real-supervisor FRP smoke client.
 *
 * Usage:
 *   TRSS_SOCKET=... TRSS_PROXY_ID_FILE=... bun trssd-frp-smoke.ts apply
 *   TRSS_SOCKET=... TRSS_PROXY_ID_FILE=... bun trssd-frp-smoke.ts stop
 */
import { readFileSync, writeFileSync } from "node:fs"
import { IpcClient } from "../src/ipc/client.ts"

const socket = process.env.TRSS_SOCKET
const idFile = process.env.TRSS_PROXY_ID_FILE
if (!socket || !idFile) {
  console.error("TRSS_SOCKET and TRSS_PROXY_ID_FILE are required")
  process.exit(2)
}

const client = await IpcClient.connect(socket)
try {
  if (process.argv[2] === "stop") {
    const proxyId = readFileSync(idFile, "utf8").trim()
    await client.call("frp.proxies.stop", { proxyId }, { timeoutMs: 5000 })
    const stopped = await client.call("frp.instances.list", {}, { timeoutMs: 5000 })
    const proxy = stopped.proxies.find((item) => item.id === proxyId)
    if (!proxy || proxy.state !== "stopped") {
      throw new Error(`proxy did not stop: ${JSON.stringify(proxy)}`)
    }
    console.log(`stopped ${proxyId}`)
    process.exit(0)
  }

  const before = await client.call("frp.instances.list", {}, { timeoutMs: 5000 })
  const endpoint = before.endpoints.find((item) => item.managed && item.state === "online")
  if (!endpoint) {
    throw new Error(`no online managed endpoint: ${JSON.stringify(before.endpoints)}`)
  }
  const applied = await client.call(
    "frp.proxies.apply",
    {
      proxy: {
        id: "",
        name: "trssd-smoke-tcp",
        type: "tcp",
        endpointId: endpoint.id,
        servicePort: 18080,
        remotePort: 18081,
        exposure: "public",
        state: "stopped",
      },
    },
    { timeoutMs: 30_000 },
  )
  if (applied.proxy.state !== "online") {
    throw new Error(`proxy not online: ${JSON.stringify(applied.proxy)}`)
  }
  writeFileSync(idFile, applied.proxy.id)
  console.log(`online ${applied.proxy.id} via ${endpoint.address}:${endpoint.port}`)
} finally {
  try {
    client.close()
  } catch {
    // already closed
  }
}
