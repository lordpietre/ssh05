package ipc

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// Frame types. Control messages are JSON; PTY bytes use the binary frame.
const (
	FrameJSON   byte = 'J'
	FrameBinary byte = 'B'
)

const (
	// MaxControlFrame bounds one JSON request/response.
	MaxControlFrame = 1 << 20
	// MaxBinaryFrame bounds one PTY chunk frame.
	MaxBinaryFrame = 4 << 20

	frameHeaderSize = 5
)

var (
	ErrFrameTooLarge  = errors.New("ipc: frame too large")
	ErrUnknownFrame   = errors.New("ipc: unknown frame type")
	ErrMalformedFrame = errors.New("ipc: malformed frame")
)

func maxFramePayload(kind byte) int {
	switch kind {
	case FrameJSON:
		return MaxControlFrame
	case FrameBinary:
		return MaxBinaryFrame
	default:
		return 0
	}
}

func writeAll(w io.Writer, data []byte) error {
	for len(data) > 0 {
		n, err := w.Write(data)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		data = data[n:]
	}
	return nil
}

func writeFrame(w io.Writer, kind byte, payload []byte) error {
	limit := maxFramePayload(kind)
	if limit == 0 {
		return ErrUnknownFrame
	}
	if len(payload) > limit {
		return fmt.Errorf("%w: %d > %d", ErrFrameTooLarge, len(payload), limit)
	}

	var header [frameHeaderSize]byte
	header[0] = kind
	binary.BigEndian.PutUint32(header[1:], uint32(len(payload)))
	if err := writeAll(w, header[:]); err != nil {
		return err
	}
	return writeAll(w, payload)
}

func readFrame(r io.Reader) (byte, []byte, error) {
	var header [frameHeaderSize]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return 0, nil, err
	}
	kind := header[0]
	limit := maxFramePayload(kind)
	if limit == 0 {
		return 0, nil, ErrUnknownFrame
	}
	size := binary.BigEndian.Uint32(header[1:])
	if int(size) > limit {
		return 0, nil, fmt.Errorf("%w: %d > %d", ErrFrameTooLarge, size, limit)
	}
	payload := make([]byte, int(size))
	if _, err := io.ReadFull(r, payload); err != nil {
		return 0, nil, err
	}
	return kind, payload, nil
}

// WriteJSONFrame encodes one control frame.
func WriteJSONFrame(w io.Writer, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return writeFrame(w, FrameJSON, payload)
}

// ReadJSONFrame decodes one control frame.
func ReadJSONFrame(r io.Reader, value any) error {
	kind, payload, err := readFrame(r)
	if err != nil {
		return err
	}
	if kind != FrameJSON {
		return ErrMalformedFrame
	}
	return json.Unmarshal(payload, value)
}

// WriteBinaryFrame encodes a session id and PTY bytes for the binary channel.
func WriteBinaryFrame(w io.Writer, sessionID string, data []byte) error {
	if len(sessionID) == 0 || len(sessionID) > 0xffff {
		return fmt.Errorf("%w: invalid session id length %d", ErrMalformedFrame, len(sessionID))
	}
	payload := make([]byte, 2+len(sessionID)+len(data))
	binary.BigEndian.PutUint16(payload[:2], uint16(len(sessionID)))
	copy(payload[2:], sessionID)
	copy(payload[2+len(sessionID):], data)
	return writeFrame(w, FrameBinary, payload)
}

// ReadBinaryFrame decodes a session id and PTY bytes from the binary channel.
func ReadBinaryFrame(payload []byte) (string, []byte, error) {
	if len(payload) < 2 {
		return "", nil, ErrMalformedFrame
	}
	size := int(binary.BigEndian.Uint16(payload[:2]))
	if size == 0 || 2+size > len(payload) {
		return "", nil, ErrMalformedFrame
	}
	return string(payload[2 : 2+size]), payload[2+size:], nil
}
