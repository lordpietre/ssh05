import { afterEach, describe, expect, test } from "bun:test"
import { mkdtempSync, rmSync } from "node:fs"
import { tmpdir } from "node:os"
import { join } from "node:path"
import type { IpcEvent, RpcRequest, RpcResponse } from "../../../../contracts/ts/index.ts"
import { IpcClient, IpcRemoteError } from "./client.ts"
import { FrameDecoder, encodeBinaryFrame, encodeControlFrame } from "./framing.ts"

const cleanups: Array<() => void> = []

afterEach(() => {
  for (const cleanup of cleanups.splice(0)) cleanup()
})

function startFakeServer(mode: "respond" | "error" | "silent" = "respond") {
  const dir = mkdtempSync(join(tmpdir(), "ssh05-ipc-"))
  const socketPath = join(dir, "control.sock")
  let requests = 0
  let serverSocket: Bun.Socket<undefined> | null = null
  const decoder = new FrameDecoder()

  const listener = Bun.listen({
    unix: socketPath,
    socket: {
      open: (socket) => {
        serverSocket = socket as Bun.Socket<undefined>
      },
      data: (socket, data) => {
        for (const frame of decoder.push(data)) {
          if (frame.kind !== "json") continue
          const request = frame.value as RpcRequest
          requests++
          if (requests === 1) {
            const response: RpcResponse = {
              protocolVersion: 1,
              requestId: request.requestId,
              result: { hosts: [], conflicts: [] },
            }
            socket.write(encodeControlFrame(response))
            const event: IpcEvent = {
              protocolVersion: 1,
              event: "session.status",
              seq: 1,
              timestamp: new Date().toISOString(),
              payload: { phase: "connected" },
            }
            socket.write(encodeControlFrame(event))
            socket.write(encodeBinaryFrame("ses-1", new TextEncoder().encode("output")))
            continue
          }
          if (mode === "silent") continue
          socket.write(
            encodeControlFrame({
              protocolVersion: 1,
              requestId: request.requestId,
              error: { code: -32601, message: mode === "error" ? "unknown method" : "unexpected" },
              result: mode === "respond" ? {} : undefined,
            } satisfies RpcResponse),
          )
        }
      },
    },
  })

  cleanups.push(() => {
    listener.stop(true)
    try {
      serverSocket?.close()
    } catch {
      // already closed
    }
    rmSync(dir, { recursive: true, force: true })
  })
  return socketPath
}

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (error: Error) => void
  const promise = new Promise<T>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

describe("IpcClient", () => {
  test("calls methods and receives events and binary data", async () => {
    const socketPath = startFakeServer()
    const client = await IpcClient.connect(socketPath)
    cleanups.push(() => client.close())

    const event = deferred<IpcEvent>()
    const binary = deferred<string>()
    client.setEventHandler((value) => event.resolve(value))
    client.setBinaryHandler((sessionId, data) => {
      binary.resolve(`${sessionId}:${new TextDecoder().decode(data)}`)
    })

    const result = await client.call("hosts.list", {}, { timeoutMs: 1000 })
    expect(result.hosts).toEqual([])

    expect((await event.promise).event).toBe("session.status")
    expect(await binary.promise).toBe("ses-1:output")
  })

  test("rejects with IpcRemoteError on a remote error", async () => {
    const socketPath = startFakeServer("error")
    const client = await IpcClient.connect(socketPath)
    cleanups.push(() => client.close())

    await client.call("hosts.list", {}, { timeoutMs: 1000 })
    await expect(client.call("hosts.list", {}, { timeoutMs: 1000 })).rejects.toBeInstanceOf(IpcRemoteError)
  })

  test("times out and rejects pending calls", async () => {
    const socketPath = startFakeServer("silent")
    const client = await IpcClient.connect(socketPath)
    cleanups.push(() => client.close())

    await client.call("hosts.list", {}, { timeoutMs: 1000 })
    await expect(client.call("hosts.list", {}, { timeoutMs: 10 })).rejects.toThrow("timeout")
  })
})
