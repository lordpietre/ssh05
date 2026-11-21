/**
 * Real-supervisor FRP smoke: managed frps + frpc with TCP, UDP, STCP and XTCP.
 * Requires the shell harness to serve HTTP on 18080 and UDP echo on 18084.
 */
import { createSocket } from "node:dgram"
import { IpcClient } from "../src/ipc/client.ts"

const socket = process.env.TRSS_SOCKET
if (!socket) {
  console.error("TRSS_SOCKET is required")
  process.exit(2)
}

const client = await IpcClient.connect(socket)

async function apply(proxy: {
  name: string
  type: "tcp" | "udp" | "stcp" | "xtcp"
  servicePort: number
  remotePort: number
  bindPort?: number
  hostProfileId?: string
}): Promise<string> {
  const list = await client.call("frp.instances.list", {}, { timeoutMs: 5000 })
  const endpoint = list.endpoints.find((item) => item.managed && item.state === "online")
  if (!endpoint) throw new Error(`no online managed endpoint: ${JSON.stringify(list.endpoints)}`)
  const applied = await client.call(
    "frp.proxies.apply",
    {
      proxy: {
        id: "",
        exposure: proxy.type === "tcp" || proxy.type === "udp" ? "public" : "private",
        state: "stopped",
        ...proxy,
        endpointId: endpoint.id,
      },
    },
    { timeoutMs: 30_000 },
  )
  if (applied.proxy.state !== "online") {
    throw new Error(`proxy not online: ${JSON.stringify(applied.proxy)}`)
  }
  return applied.proxy.id
}

async function stop(proxyId: string): Promise<void> {
  await client.call("frp.proxies.stop", { proxyId }, { timeoutMs: 5000 })
  const list = await client.call("frp.instances.list", {}, { timeoutMs: 5000 })
  const proxy = list.proxies.find((item) => item.id === proxyId)
  if (!proxy || proxy.state !== "stopped") {
    throw new Error(`proxy did not stop: ${JSON.stringify(proxy)}`)
  }
}

async function waitFetch(url: string, expected: string, timeoutMs = 10_000): Promise<void> {
  const deadline = Date.now() + timeoutMs
  let lastError = ""
  while (Date.now() < deadline) {
    try {
      const response = await fetch(url, { signal: AbortSignal.timeout(1500) })
      const text = await response.text()
      if (text.includes(expected)) return
      lastError = `unexpected payload: ${text.slice(0, 80)}`
    } catch (error) {
      lastError = String(error)
    }
    await Bun.sleep(200)
  }
  throw new Error(`${url} not reachable: ${lastError}`)
}

async function waitUnreachable(url: string, timeoutMs = 3000): Promise<void> {
  const deadline = Date.now() + timeoutMs
  while (Date.now() < deadline) {
    try {
      await fetch(url, { signal: AbortSignal.timeout(400) })
      await Bun.sleep(200)
    } catch {
      return
    }
  }
  throw new Error(`${url} is still reachable after stop`)
}

function udpProbe(port: number, payload: string, timeoutMs = 1500): Promise<string> {
  return new Promise((resolve, reject) => {
    const socket = createSocket("udp4")
    const timer = setTimeout(() => {
      socket.close()
      reject(new Error("udp timeout"))
    }, timeoutMs)
    socket.on("message", (message) => {
      clearTimeout(timer)
      socket.close()
      resolve(message.toString())
    })
    socket.on("error", (error) => {
      clearTimeout(timer)
      socket.close()
      reject(error)
    })
    socket.send(payload, port, "127.0.0.1")
  })
}

async function waitUDP(port: number, payload: string, expected: string): Promise<void> {
  const deadline = Date.now() + 10_000
  let lastError = ""
  while (Date.now() < deadline) {
    try {
      if ((await udpProbe(port, payload)) === expected) return
    } catch (error) {
      lastError = String(error)
    }
    await Bun.sleep(200)
  }
  throw new Error(`udp ${port} not reachable: ${lastError}`)
}

try {
  const tcp = await apply({
    name: "trssd-smoke-tcp",
    type: "tcp",
    servicePort: 18080,
    remotePort: 18081,
  })
  console.log(`tcp online ${tcp}`)
  await waitFetch("http://127.0.0.1:18081/hello.txt", "TRSS_FRP_SUPERVISOR_OK")
  await stop(tcp)
  await waitUnreachable("http://127.0.0.1:18081/hello.txt")

  const udp = await apply({
    name: "trssd-smoke-udp",
    type: "udp",
    servicePort: 18084,
    remotePort: 18085,
  })
  console.log(`udp online ${udp}`)
  await waitUDP(18085, "trssd-udp-ping", "trssd-udp-ping")
  await stop(udp)

  const stcp = await apply({
    name: "trssd-smoke-stcp",
    type: "stcp",
    servicePort: 18080,
    remotePort: 0,
    bindPort: 18082,
    hostProfileId: "smoke-stcp",
  })
  console.log(`stcp online ${stcp}`)
  await waitFetch("http://127.0.0.1:18082/hello.txt", "TRSS_FRP_SUPERVISOR_OK")
  const route = await client.call("routes.plan", { hostProfileId: "smoke-stcp" }, { timeoutMs: 3000 })
  if (!(route.selected ?? "").startsWith("frp:")) {
    throw new Error(`missing FRP route: ${JSON.stringify(route)}`)
  }
  await stop(stcp)

  const xtcp = await apply({
    name: "trssd-smoke-xtcp",
    type: "xtcp",
    servicePort: 18080,
    remotePort: 0,
    bindPort: 18083,
  })
  console.log(`xtcp online ${xtcp}`)
  await waitFetch("http://127.0.0.1:18083/hello.txt", "TRSS_FRP_SUPERVISOR_OK")
  await stop(xtcp)

  console.log("PASS: TCP, UDP, STCP and XTCP through the supervisor")
} finally {
  try {
    client.close()
  } catch {
    // already closed
  }
}
