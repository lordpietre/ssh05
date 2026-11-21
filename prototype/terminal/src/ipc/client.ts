import type {
  IpcEvent,
  Method,
  ParamsOf,
  ResultOf,
  RpcRequest,
  RpcResponse,
} from "../../../../contracts/ts/index.ts"
import { PROTOCOL_VERSION } from "../../../../contracts/ts/index.ts"
import { FrameDecoder, encodeBinaryFrame, encodeControlFrame } from "./framing.ts"

export class IpcRemoteError extends Error {
  readonly code: number
  readonly data?: unknown

  constructor(code: number, message: string, data?: unknown) {
    super(message)
    this.name = "IpcRemoteError"
    this.code = code
    this.data = data
  }
}

type CallOptions = {
  timeoutMs?: number
  signal?: AbortSignal
}

type Pending = {
  resolve: (value: unknown) => void
  reject: (error: Error) => void
  timer?: ReturnType<typeof setTimeout>
  abort?: () => void
  signal?: AbortSignal
}

export type EventHandler = (event: IpcEvent) => void
export type BinaryHandler = (sessionId: string, data: Uint8Array) => void

/**
 * Typed client for the local supervisor. It mirrors internal/ipc: JSON control
 * frames, JSON events and a separate binary PTY channel.
 */
export class IpcClient {
  private socket: Bun.Socket<undefined> | null = null
  private readonly decoder = new FrameDecoder()
  private readonly pending = new Map<string, Pending>()
  private nextId = 0
  private closed = false
  private eventHandler?: EventHandler
  private binaryHandler?: BinaryHandler

  static async connect(path: string): Promise<IpcClient> {
    const client = new IpcClient()
    const socket = await Bun.connect({
      unix: path,
      socket: {
        open: (socket) => {
          client.socket = socket as Bun.Socket<undefined>
        },
        data: (_socket, data) => client.onData(data),
        close: () => client.closeWithError(new Error("ipc: connection closed")),
        error: (_socket, error) => client.closeWithError(error),
      },
    })
    client.socket = socket as Bun.Socket<undefined>
    return client
  }

  setEventHandler(handler: EventHandler): void {
    this.eventHandler = handler
  }

  setBinaryHandler(handler: BinaryHandler): void {
    this.binaryHandler = handler
  }

  call<M extends Method>(method: M, params: ParamsOf<M>, options: CallOptions = {}): Promise<ResultOf<M>> {
    if (this.closed) return Promise.reject(new Error("ipc: client closed"))
    if (options.signal?.aborted) return Promise.reject(abortError())

    const requestId = `req-${++this.nextId}`
    const request: RpcRequest<ParamsOf<M>> = {
      protocolVersion: PROTOCOL_VERSION,
      requestId,
      method,
      params,
      timestamp: new Date().toISOString(),
    }

    return new Promise<ResultOf<M>>((resolve, reject) => {
      const pending: Pending = {
        resolve: (value) => resolve(value as ResultOf<M>),
        reject,
      }
      if (options.timeoutMs && options.timeoutMs > 0) {
        pending.timer = setTimeout(() => {
          this.pending.delete(requestId)
          reject(new Error(`ipc: timeout after ${options.timeoutMs}ms`))
        }, options.timeoutMs)
      }
      if (options.signal) {
        const abort = () => {
          this.pending.delete(requestId)
          cleanup(pending)
          reject(abortError())
        }
        pending.abort = abort
        pending.signal = options.signal
        options.signal.addEventListener("abort", abort, { once: true })
      }
      this.pending.set(requestId, pending)
      try {
        this.send(encodeControlFrame(request))
      } catch (error) {
        this.pending.delete(requestId)
        cleanup(pending)
        reject(error as Error)
      }
    })
  }

  sendBinary(sessionId: string, data: Uint8Array): void {
    if (this.closed) throw new Error("ipc: client closed")
    this.send(encodeBinaryFrame(sessionId, data))
  }

  close(): void {
    if (this.closed) return
    this.closed = true
    try {
      this.socket?.close()
    } finally {
      this.rejectAll(new Error("ipc: client closed"))
    }
  }

  private send(frame: Uint8Array): void {
    if (!this.socket) throw new Error("ipc: socket not connected")
    this.socket.write(frame)
  }

  private onData(data: Uint8Array): void {
    for (const frame of this.decoder.push(data)) {
      if (frame.kind === "binary") {
        this.binaryHandler?.(frame.sessionId, frame.data)
        continue
      }
      const value = frame.value as Partial<RpcResponse & IpcEvent>
      if (value?.event) {
        const event = value as IpcEvent
        if (event.protocolVersion !== PROTOCOL_VERSION) {
          this.closeWithError(new Error(`ipc: event protocol version ${event.protocolVersion}`))
          return
        }
        this.eventHandler?.(event)
        continue
      }
      if (!value?.requestId) {
        this.closeWithError(new Error("ipc: response without requestId"))
        return
      }
      const response = value as RpcResponse
      const pending = this.pending.get(response.requestId)
      if (!pending) continue
      this.pending.delete(response.requestId)
      cleanup(pending)
      if (response.error) {
        pending.reject(new IpcRemoteError(response.error.code, response.error.message, response.error.data))
        continue
      }
      pending.resolve(response.result)
    }
  }

  private closeWithError(error: Error): void {
    if (this.closed) return
    this.closed = true
    try {
      this.socket?.close()
    } finally {
      this.rejectAll(error)
    }
  }

  private rejectAll(error: Error): void {
    for (const pending of this.pending.values()) {
      cleanup(pending)
      pending.reject(error)
    }
    this.pending.clear()
  }
}

function cleanup(pending: Pending): void {
  if (pending.timer) clearTimeout(pending.timer)
  if (pending.abort && pending.signal) {
    pending.signal.removeEventListener("abort", pending.abort)
  }
}

function abortError(): Error {
  const error = new Error("ipc: aborted")
  error.name = "AbortError"
  return error
}
