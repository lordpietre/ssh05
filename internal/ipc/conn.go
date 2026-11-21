package ipc

import (
	"bufio"
	"net"
	"sync"

	"github.com/trzsz/trzsz-ssh/contracts"
)

// BinaryHandler receives a PTY chunk and its session id.
type BinaryHandler func(sessionID string, data []byte)

// Conn is the server-side view of one authenticated control connection. A
// handler may keep it to stream PTY output after the JSON response has been
// written; writes are serialized with responses on the same connection.
type Conn struct {
	conn   net.Conn
	writer *bufio.Writer
	done   chan struct{}

	writeMu sync.Mutex

	binaryMu      sync.Mutex
	binaryHandler BinaryHandler
}

func newConn(conn net.Conn, writer *bufio.Writer) *Conn {
	return &Conn{conn: conn, writer: writer, done: make(chan struct{})}
}

// Done is closed when the underlying connection ends.
func (c *Conn) Done() <-chan struct{} {
	return c.done
}

func (c *Conn) close() {
	select {
	case <-c.done:
	default:
		close(c.done)
	}
}

// SendEvent writes an unsolicited JSON event to the client.
func (c *Conn) SendEvent(event contracts.Event) error {
	return c.writeJSON(event)
}

// SendBinary writes one PTY frame to the client.
func (c *Conn) SendBinary(sessionID string, data []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if err := WriteBinaryFrame(c.writer, sessionID, data); err != nil {
		return err
	}
	return c.writer.Flush()
}

// SetBinaryHandler installs the callback for inbound PTY bytes.
func (c *Conn) SetBinaryHandler(handler BinaryHandler) {
	c.binaryMu.Lock()
	c.binaryHandler = handler
	c.binaryMu.Unlock()
}

func (c *Conn) dispatchBinary(sessionID string, data []byte) {
	c.binaryMu.Lock()
	handler := c.binaryHandler
	c.binaryMu.Unlock()
	if handler != nil {
		handler(sessionID, data)
	}
}

func (c *Conn) writeJSON(value any) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if err := WriteJSONFrame(c.writer, value); err != nil {
		return err
	}
	return c.writer.Flush()
}
