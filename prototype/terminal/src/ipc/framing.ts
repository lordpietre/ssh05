/**
 * Wire framing shared with internal/ipc.
 *
 * [kind:1][length:uint32][payload]
 *   'J' JSON control
 *   'B' binary: [sessionIdLength:uint16][sessionId][data]
 */

export const FRAME_JSON = 0x4a // 'J'
export const FRAME_BINARY = 0x42 // 'B'
export const MAX_CONTROL_FRAME = 1 << 20
export const MAX_BINARY_FRAME = 4 << 20

const HEADER_SIZE = 5
const encoder = new TextEncoder()
const decoder = new TextDecoder()

export type DecodedFrame =
  | { kind: "json"; value: unknown }
  | { kind: "binary"; sessionId: string; data: Uint8Array }

export class FrameError extends Error {}

function maxPayload(kind: number): number {
  if (kind === FRAME_JSON) return MAX_CONTROL_FRAME
  if (kind === FRAME_BINARY) return MAX_BINARY_FRAME
  throw new FrameError(`unknown frame kind ${kind}`)
}

function encodeFrame(kind: number, payload: Uint8Array): Uint8Array {
  const limit = maxPayload(kind)
  if (payload.length > limit) {
    throw new FrameError(`frame too large: ${payload.length} > ${limit}`)
  }
  const frame = new Uint8Array(HEADER_SIZE + payload.length)
  frame[0] = kind
  new DataView(frame.buffer).setUint32(1, payload.length, false)
  frame.set(payload, HEADER_SIZE)
  return frame
}

export function encodeControlFrame(value: unknown): Uint8Array {
  return encodeFrame(FRAME_JSON, encoder.encode(JSON.stringify(value)))
}

export function encodeBinaryFrame(sessionId: string, data: Uint8Array): Uint8Array {
  const id = encoder.encode(sessionId)
  if (id.length === 0 || id.length > 0xffff) {
    throw new FrameError(`invalid session id length ${id.length}`)
  }
  const payload = new Uint8Array(2 + id.length + data.length)
  new DataView(payload.buffer).setUint16(0, id.length, false)
  payload.set(id, 2)
  payload.set(data, 2 + id.length)
  return encodeFrame(FRAME_BINARY, payload)
}

export class FrameDecoder {
  private buffer = new Uint8Array(0)

  push(chunk: Uint8Array): DecodedFrame[] {
    if (chunk.length > 0) {
      const merged = new Uint8Array(this.buffer.length + chunk.length)
      merged.set(this.buffer)
      merged.set(chunk, this.buffer.length)
      this.buffer = merged
    }

    const frames: DecodedFrame[] = []
    while (this.buffer.length >= HEADER_SIZE) {
      const kind = this.buffer[0]
      const size = new DataView(this.buffer.buffer, this.buffer.byteOffset, this.buffer.byteLength).getUint32(
        1,
        false,
      )
      const limit = maxPayload(kind)
      if (size > limit) throw new FrameError(`frame too large: ${size} > ${limit}`)
      if (this.buffer.length < HEADER_SIZE + size) break
      const payload = this.buffer.slice(HEADER_SIZE, HEADER_SIZE + size)
      this.buffer = this.buffer.slice(HEADER_SIZE + size)
      frames.push(this.decode(kind, payload))
    }
    return frames
  }

  private decode(kind: number, payload: Uint8Array): DecodedFrame {
    if (kind === FRAME_JSON) {
      try {
        return { kind: "json", value: JSON.parse(decoder.decode(payload)) }
      } catch (error) {
        throw new FrameError(`invalid JSON frame: ${String(error)}`)
      }
    }
    if (payload.length < 2) throw new FrameError("malformed binary frame")
    const idLength = new DataView(payload.buffer, payload.byteOffset, payload.byteLength).getUint16(0, false)
    if (idLength === 0 || 2 + idLength > payload.length) {
      throw new FrameError("malformed binary frame")
    }
    return {
      kind: "binary",
      sessionId: decoder.decode(payload.slice(2, 2 + idLength)),
      data: payload.slice(2 + idLength),
    }
  }
}
