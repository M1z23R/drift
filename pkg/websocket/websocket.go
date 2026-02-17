package websocket

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/m1z23r/drift/pkg/drift"
)

// JSON helpers
func jsonMarshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

func jsonUnmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

// MessageType represents the type of WebSocket message
type MessageType int

const (
	TextMessage   MessageType = OpText
	BinaryMessage MessageType = OpBinary
	CloseMessage  MessageType = OpClose
	PingMessage   MessageType = OpPing
	PongMessage   MessageType = OpPong
)

// Conn represents a WebSocket connection
type Conn struct {
	conn       net.Conn
	reader     *bufio.Reader
	writer     *bufio.Writer
	writeMu    sync.Mutex
	readMu     sync.Mutex
	closed     bool
	closeMu    sync.Mutex
	readLimit  int64
	closeCode  int
	closeText  string

	// For fragmented messages
	fragmentBuffer []byte
	fragmentOpcode byte
}

// Upgrader specifies parameters for upgrading an HTTP connection
type Upgrader struct {
	// ReadBufferSize specifies the buffer size for the reader
	ReadBufferSize int
	// WriteBufferSize specifies the buffer size for the writer
	WriteBufferSize int
	// ReadLimit is the maximum size of a message (0 = default 32MB)
	ReadLimit int64
	// CheckOrigin validates the request origin. If nil, origin is not checked.
	CheckOrigin func(r *http.Request) bool
	// Subprotocols specifies the server's supported protocols in preference order
	Subprotocols []string
}

var DefaultUpgrader = &Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	ReadLimit:       defaultReadLimit,
}

// Upgrade upgrades an HTTP connection to a WebSocket connection using default settings
func Upgrade(c *drift.Context) (*Conn, error) {
	return DefaultUpgrader.Upgrade(c)
}

// Upgrade upgrades an HTTP connection to a WebSocket connection
func (u *Upgrader) Upgrade(c *drift.Context) (*Conn, error) {
	r := c.Request
	w := c.Response

	// Validate the handshake
	if err := validateHandshake(r); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return nil, err
	}

	// Check origin if configured
	if u.CheckOrigin != nil && !u.CheckOrigin(r) {
		http.Error(w, "origin not allowed", http.StatusForbidden)
		return nil, errors.New("websocket: origin not allowed")
	}

	// Get the hijacker
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, ErrHijackNotSupport.Error(), http.StatusInternalServerError)
		return nil, ErrHijackNotSupport
	}

	// Compute accept key
	key := r.Header.Get("Sec-WebSocket-Key")
	acceptKey := computeAcceptKey(key)

	// Negotiate subprotocol
	subprotocol := u.negotiateSubprotocol(r)

	// Hijack the connection
	conn, bufrw, err := hj.Hijack()
	if err != nil {
		return nil, err
	}

	// Write the upgrade response
	response := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + acceptKey + "\r\n"

	if subprotocol != "" {
		response += "Sec-WebSocket-Protocol: " + subprotocol + "\r\n"
	}
	response += "\r\n"

	if _, err := bufrw.WriteString(response); err != nil {
		conn.Close()
		return nil, err
	}
	if err := bufrw.Flush(); err != nil {
		conn.Close()
		return nil, err
	}

	// Set buffer sizes
	readBufSize := u.ReadBufferSize
	if readBufSize == 0 {
		readBufSize = 4096
	}
	writeBufSize := u.WriteBufferSize
	if writeBufSize == 0 {
		writeBufSize = 4096
	}

	readLimit := u.ReadLimit
	if readLimit == 0 {
		readLimit = defaultReadLimit
	}

	ws := &Conn{
		conn:      conn,
		reader:    bufio.NewReaderSize(conn, readBufSize),
		writer:    bufio.NewWriterSize(conn, writeBufSize),
		readLimit: readLimit,
	}

	return ws, nil
}

// negotiateSubprotocol selects a subprotocol from client's preferences
func (u *Upgrader) negotiateSubprotocol(r *http.Request) string {
	if len(u.Subprotocols) == 0 {
		return ""
	}

	clientProtocols := r.Header.Get("Sec-WebSocket-Protocol")
	if clientProtocols == "" {
		return ""
	}

	for _, serverProto := range u.Subprotocols {
		if containsIgnoreCase(clientProtocols, serverProto) {
			return serverProto
		}
	}
	return ""
}

// ReadMessage reads a complete message from the connection
// It handles fragmentation automatically
func (c *Conn) ReadMessage() (MessageType, []byte, error) {
	c.readMu.Lock()
	defer c.readMu.Unlock()

	for {
		frame, err := readFrame(c.reader, c.readLimit)
		if err != nil {
			return 0, nil, err
		}

		// Handle control frames
		switch frame.opcode {
		case OpClose:
			c.handleClose(frame.payload)
			return CloseMessage, frame.payload, ErrConnectionClosed

		case OpPing:
			if err := c.writePong(frame.payload); err != nil {
				return 0, nil, err
			}
			continue

		case OpPong:
			// Pong received, continue reading
			continue
		}

		// Handle data frames
		if frame.opcode == OpContinuation {
			if c.fragmentBuffer == nil {
				return 0, nil, ErrContinuation
			}
			c.fragmentBuffer = append(c.fragmentBuffer, frame.payload...)
			if frame.fin {
				// Message complete
				msg := c.fragmentBuffer
				opcode := c.fragmentOpcode
				c.fragmentBuffer = nil
				c.fragmentOpcode = 0
				return MessageType(opcode), msg, nil
			}
			continue
		}

		// New message
		if frame.opcode == OpText || frame.opcode == OpBinary {
			if c.fragmentBuffer != nil {
				return 0, nil, ErrNotContinuation
			}
			if frame.fin {
				// Complete message in single frame
				return MessageType(frame.opcode), frame.payload, nil
			}
			// Start of fragmented message
			c.fragmentOpcode = frame.opcode
			c.fragmentBuffer = frame.payload
			continue
		}

		return 0, nil, ErrInvalidOpcode
	}
}

// WriteMessage writes a complete message to the connection
func (c *Conn) WriteMessage(messageType MessageType, data []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	c.closeMu.Lock()
	if c.closed {
		c.closeMu.Unlock()
		return ErrConnectionClosed
	}
	c.closeMu.Unlock()

	return writeFrame(c.writer, byte(messageType), data, true)
}

// WriteText writes a text message
func (c *Conn) WriteText(text string) error {
	return c.WriteMessage(TextMessage, []byte(text))
}

// WriteBinary writes a binary message
func (c *Conn) WriteBinary(data []byte) error {
	return c.WriteMessage(BinaryMessage, data)
}

// WriteJSON writes a JSON message
func (c *Conn) WriteJSON(v any) error {
	data, err := jsonMarshal(v)
	if err != nil {
		return err
	}
	return c.WriteText(string(data))
}

// ReadJSON reads a JSON message
func (c *Conn) ReadJSON(v any) error {
	_, data, err := c.ReadMessage()
	if err != nil {
		return err
	}
	return jsonUnmarshal(data, v)
}

// Ping sends a ping message
func (c *Conn) Ping(data []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	if len(data) > maxControlPayload {
		data = data[:maxControlPayload]
	}
	return writeFrame(c.writer, OpPing, data, true)
}

// writePong sends a pong message (response to ping)
func (c *Conn) writePong(data []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	return writeFrame(c.writer, OpPong, data, true)
}

// Close closes the WebSocket connection with a status code and text
func (c *Conn) Close(code int, text string) error {
	c.closeMu.Lock()
	if c.closed {
		c.closeMu.Unlock()
		return nil
	}
	c.closed = true
	c.closeMu.Unlock()

	// Build close frame payload: 2-byte code + text
	payload := make([]byte, 2+len(text))
	binary.BigEndian.PutUint16(payload, uint16(code))
	copy(payload[2:], text)

	c.writeMu.Lock()
	writeFrame(c.writer, OpClose, payload, true)
	c.writeMu.Unlock()

	// Give the peer time to process the close frame
	c.conn.SetReadDeadline(time.Now().Add(time.Second))
	c.conn.Close()
	return nil
}

// handleClose processes an incoming close frame
func (c *Conn) handleClose(payload []byte) {
	c.closeMu.Lock()
	defer c.closeMu.Unlock()

	if len(payload) >= 2 {
		c.closeCode = int(binary.BigEndian.Uint16(payload))
		c.closeText = string(payload[2:])
	}

	if !c.closed {
		c.closed = true
		// Echo close frame back
		c.writeMu.Lock()
		writeFrame(c.writer, OpClose, payload, true)
		c.writeMu.Unlock()
		c.conn.Close()
	}
}

// CloseCode returns the close code received from peer (0 if not closed)
func (c *Conn) CloseCode() int {
	c.closeMu.Lock()
	defer c.closeMu.Unlock()
	return c.closeCode
}

// CloseText returns the close reason received from peer
func (c *Conn) CloseText() string {
	c.closeMu.Lock()
	defer c.closeMu.Unlock()
	return c.closeText
}

// SetReadLimit sets the maximum message size
func (c *Conn) SetReadLimit(limit int64) {
	c.readLimit = limit
}

// SetReadDeadline sets the read deadline on the underlying connection
func (c *Conn) SetReadDeadline(t time.Time) error {
	return c.conn.SetReadDeadline(t)
}

// SetWriteDeadline sets the write deadline on the underlying connection
func (c *Conn) SetWriteDeadline(t time.Time) error {
	return c.conn.SetWriteDeadline(t)
}

// RemoteAddr returns the remote network address
func (c *Conn) RemoteAddr() net.Addr {
	return c.conn.RemoteAddr()
}

// LocalAddr returns the local network address
func (c *Conn) LocalAddr() net.Addr {
	return c.conn.LocalAddr()
}

// Underlying returns the underlying net.Conn (use with caution)
func (c *Conn) Underlying() net.Conn {
	return c.conn
}
