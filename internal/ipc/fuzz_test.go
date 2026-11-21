package ipc

import (
	"bytes"
	"testing"
)

func FuzzReadFrame(f *testing.F) {
	f.Add([]byte{FrameJSON, 0, 0, 0, 2, '{', '}'})
	f.Add([]byte{FrameBinary, 0, 0, 0, 4, 0, 1, 'a', 'b'})
	f.Add([]byte{'X', 0xff, 0xff, 0xff, 0xff})
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _, _ = readFrame(bytes.NewReader(data))
	})
}

func FuzzReadBinaryFrame(f *testing.F) {
	f.Add([]byte{0, 3, 's', 'e', 's', 'x'})
	f.Add([]byte{0, 0})
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _, _ = ReadBinaryFrame(data)
	})
}

func FuzzWriteFrameRoundTrip(f *testing.F) {
	f.Add([]byte("payload"), false)
	f.Fuzz(func(t *testing.T, data []byte, binary bool) {
		kind := FrameJSON
		if binary {
			kind = FrameBinary
		}
		if len(data) > maxFramePayload(kind) {
			t.Skip()
		}
		var buffer bytes.Buffer
		if err := writeFrame(&buffer, kind, data); err != nil {
			t.Fatalf("writeFrame: %v", err)
		}
		gotKind, payload, err := readFrame(&buffer)
		if err != nil {
			t.Fatalf("readFrame: %v", err)
		}
		if gotKind != kind || !bytes.Equal(payload, data) {
			t.Fatalf("round trip mismatch")
		}
	})
}
