package ssh05

import (
	"bytes"
	"encoding/base64"
	"sync"
	"sync/atomic"

	"github.com/atotto/clipboard"
)

var (
	osc52Enabled   atomic.Bool
	osc52Buf       bytes.Buffer
	osc52Buffering atomic.Bool
	osc52Mu        sync.Mutex
)

func init() {
	osc52Enabled.Store(true)
}

func setOSC52Enabled(enabled bool) {
	osc52Enabled.Store(enabled)
	if !enabled {
		osc52Mu.Lock()
		osc52Buffering.Store(false)
		osc52Buf.Reset()
		osc52Mu.Unlock()
	}
}

func isOSC52Enabled() bool {
	return osc52Enabled.Load()
}

func detectAndHandleOSC52(buf []byte) {
	if !osc52Enabled.Load() {
		return
	}

	osc52Mu.Lock()
	defer osc52Mu.Unlock()

	if !osc52Enabled.Load() {
		return
	}

	if osc52Buffering.Load() {
		buf = appendPartialOSC52Locked(buf)
		if len(buf) == 0 {
			return
		}
	}

	for len(buf) > 0 {
		pos := bytes.Index(buf, []byte{0x1b, ']', '5', '2', ';'})
		if pos < 0 {
			return
		}
		buf = buf[pos+5:]
		if len(buf) < 2 {
			return
		}
		if (buf[0] != 'c' && buf[0] != 'p') || buf[1] != ';' {
			buf = buf[2:]
			continue
		}
		buf = buf[2:]

		end := bytes.IndexAny(buf, "\a\x1b")
		if end < 0 {
			osc52Buffering.Store(true)
			osc52Buf.Reset()
			osc52Buf.Write(buf)
			return
		}

		writeBase64ToClipboard(buf[:end])
		if buf[end] == '\a' {
			buf = buf[end+1:]
		} else {
			buf = buf[end+1:]
			if len(buf) > 0 && buf[0] == '\\' {
				buf = buf[1:]
			}
		}
	}
}

func appendPartialOSC52Locked(buf []byte) []byte {
	end := bytes.IndexAny(buf, "\a\x1b")
	if end < 0 {
		if osc52Buf.Len()+len(buf) > 100000 {
			osc52Buffering.Store(false)
			osc52Buf.Reset()
			return nil
		}
		osc52Buf.Write(buf)
		return nil
	}

	osc52Buf.Write(buf[:end])
	writeBase64ToClipboard(osc52Buf.Bytes())
	osc52Buf.Reset()
	osc52Buffering.Store(false)

	if buf[end] == '\a' {
		return buf[end+1:]
	}
	if len(buf) > end+1 && buf[end+1] == '\\' {
		return buf[end+2:]
	}
	return buf[end+1:]
}

func writeBase64ToClipboard(data []byte) {
	text, err := base64.StdEncoding.DecodeString(string(data))
	if err != nil {
		return
	}
	_ = clipboard.WriteAll(string(text))
}
