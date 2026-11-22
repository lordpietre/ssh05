package ipc

import (
	"bytes"
	"testing"
)

func BenchmarkBinaryFrameRoundTrip64K(b *testing.B) {
	payload := make([]byte, 64<<10)
	var buffer bytes.Buffer
	b.SetBytes(int64(len(payload)))
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		buffer.Reset()
		if err := WriteBinaryFrame(&buffer, "ses-1", payload); err != nil {
			b.Fatal(err)
		}
		kind, data, err := readFrame(bytes.NewReader(buffer.Bytes()))
		if err != nil || kind != FrameBinary {
			b.Fatalf("readFrame = %v/%v", kind, err)
		}
		if _, body, err := ReadBinaryFrame(data); err != nil || len(body) != len(payload) {
			b.Fatalf("ReadBinaryFrame = %d/%v", len(body), err)
		}
	}
}

func BenchmarkJSONEnvelopeRoundTrip(b *testing.B) {
	value := map[string]any{
		"jsonrpc": "2.0",
		"id":      "req-123",
		"method":  "sessions.open",
		"params":  map[string]any{"hostProfileId": "lab", "tmuxSession": "ssh05-lab"},
	}
	var buffer bytes.Buffer
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		buffer.Reset()
		if err := WriteJSONFrame(&buffer, value); err != nil {
			b.Fatal(err)
		}
		var decoded map[string]any
		if err := ReadJSONFrame(bytes.NewReader(buffer.Bytes()), &decoded); err != nil {
			b.Fatal(err)
		}
	}
}
