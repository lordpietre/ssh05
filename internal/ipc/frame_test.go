package ipc

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"

	"github.com/lordpietre/ssh05/contracts"
)

func TestJSONFrameRoundTrip(t *testing.T) {
	var buffer bytes.Buffer
	request := contracts.RPCRequest{
		ProtocolVersion: contracts.ProtocolVersion,
		RequestID:       "req-1",
		Method:          contracts.MethodHostsList,
	}
	if err := WriteJSONFrame(&buffer, request); err != nil {
		t.Fatalf("WriteJSONFrame: %v", err)
	}
	kind, payload, err := readFrame(&buffer)
	if err != nil {
		t.Fatalf("readFrame: %v", err)
	}
	if kind != FrameJSON {
		t.Fatalf("kind = %q, want %q", kind, FrameJSON)
	}
	if len(payload) == 0 {
		t.Fatal("empty payload")
	}

	var second bytes.Buffer
	if err := WriteJSONFrame(&second, request); err != nil {
		t.Fatalf("WriteJSONFrame second: %v", err)
	}
	var decoded contracts.RPCRequest
	if err := ReadJSONFrame(&second, &decoded); err != nil {
		t.Fatalf("ReadJSONFrame: %v", err)
	}
	if decoded.RequestID != request.RequestID || decoded.Method != request.Method {
		t.Fatalf("decoded = %+v", decoded)
	}
}

func TestFrameTooLarge(t *testing.T) {
	payload := make([]byte, MaxControlFrame+1)
	if err := writeFrame(&bytes.Buffer{}, FrameJSON, payload); !errors.Is(err, ErrFrameTooLarge) {
		t.Fatalf("error = %v, want ErrFrameTooLarge", err)
	}
}

func TestUnknownFrameType(t *testing.T) {
	if err := writeFrame(&bytes.Buffer{}, 'X', nil); !errors.Is(err, ErrUnknownFrame) {
		t.Fatalf("error = %v, want ErrUnknownFrame", err)
	}
	var header [frameHeaderSize]byte
	header[0] = 'X'
	if _, _, err := readFrame(bytes.NewReader(header[:])); !errors.Is(err, ErrUnknownFrame) {
		t.Fatalf("error = %v, want ErrUnknownFrame", err)
	}
}

func TestOversizedFrameHeaderIsRejected(t *testing.T) {
	var header [frameHeaderSize]byte
	header[0] = FrameJSON
	binary.BigEndian.PutUint32(header[1:], MaxControlFrame+1)
	if _, _, err := readFrame(bytes.NewReader(header[:])); !errors.Is(err, ErrFrameTooLarge) {
		t.Fatalf("error = %v, want ErrFrameTooLarge", err)
	}
}

func TestBinaryFrameRoundTrip(t *testing.T) {
	var buffer bytes.Buffer
	if err := WriteBinaryFrame(&buffer, "ses-7", []byte("hello")); err != nil {
		t.Fatalf("WriteBinaryFrame: %v", err)
	}
	kind, payload, err := readFrame(&buffer)
	if err != nil {
		t.Fatalf("readFrame: %v", err)
	}
	if kind != FrameBinary {
		t.Fatalf("kind = %q, want %q", kind, FrameBinary)
	}
	sessionID, data, err := ReadBinaryFrame(payload)
	if err != nil {
		t.Fatalf("ReadBinaryFrame: %v", err)
	}
	if sessionID != "ses-7" || string(data) != "hello" {
		t.Fatalf("got session=%q data=%q", sessionID, data)
	}
}

func TestMalformedBinaryFrame(t *testing.T) {
	for _, payload := range [][]byte{
		nil,
		{0},
		{0, 0},
		{0, 10, 'a'},
	} {
		if _, _, err := ReadBinaryFrame(payload); !errors.Is(err, ErrMalformedFrame) {
			t.Fatalf("payload %v: error = %v, want ErrMalformedFrame", payload, err)
		}
	}
}
