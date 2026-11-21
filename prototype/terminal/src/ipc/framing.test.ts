import { describe, expect, test } from "bun:test"
import {
  FrameDecoder,
  FrameError,
  MAX_CONTROL_FRAME,
  encodeBinaryFrame,
  encodeControlFrame,
} from "./framing.ts"

describe("framing", () => {
  test("round-trips a JSON control frame", () => {
    const frame = encodeControlFrame({ protocolVersion: 1, requestId: "req-1" })
    const decoded = new FrameDecoder().push(frame)
    expect(decoded).toHaveLength(1)
    expect(decoded[0]).toEqual({
      kind: "json",
      value: { protocolVersion: 1, requestId: "req-1" },
    })
  })

  test("round-trips a binary PTY frame", () => {
    const frame = encodeBinaryFrame("ses-1", new TextEncoder().encode("hello"))
    const decoded = new FrameDecoder().push(frame)
    expect(decoded).toHaveLength(1)
    const first = decoded[0]
    expect(first.kind).toBe("binary")
    if (first.kind !== "binary") throw new Error("unreachable")
    expect(first.sessionId).toBe("ses-1")
    expect(new TextDecoder().decode(first.data)).toBe("hello")
  })

  test("handles partial frames across chunks", () => {
    const frame = encodeBinaryFrame("ses-1", new TextEncoder().encode("hello"))
    const decoder = new FrameDecoder()
    expect(decoder.push(frame.slice(0, 3))).toHaveLength(0)
    expect(decoder.push(frame.slice(3, 7))).toHaveLength(0)
    const decoded = decoder.push(frame.slice(7))
    expect(decoded).toHaveLength(1)
  })

  test("decodes multiple frames from one chunk", () => {
    const a = encodeControlFrame({ n: 1 })
    const b = encodeControlFrame({ n: 2 })
    const merged = new Uint8Array(a.length + b.length)
    merged.set(a)
    merged.set(b, a.length)
    const decoded = new FrameDecoder().push(merged)
    expect(decoded).toHaveLength(2)
    expect(decoded[0]).toEqual({ kind: "json", value: { n: 1 } })
    expect(decoded[1]).toEqual({ kind: "json", value: { n: 2 } })
  })

  test("rejects oversized frames", () => {
    const payload = new Uint8Array(MAX_CONTROL_FRAME + 1)
    expect(() => encodeControlFrame(payload)).toThrow(FrameError)
  })

  test("rejects malformed binary payloads", () => {
    const payload = new Uint8Array(2)
    const frame = new Uint8Array(5 + payload.length)
    frame[0] = 0x42
    new DataView(frame.buffer).setUint32(1, payload.length, false)
    frame.set(payload, 5)
    expect(() => new FrameDecoder().push(frame)).toThrow(FrameError)
  })
})
