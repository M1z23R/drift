package websocket

import (
	"bufio"
	"encoding/binary"
	"net"
	"sync"
	"testing"
	"time"
)

func newTestConn(t *testing.T) (*Conn, net.Conn) {
	t.Helper()
	server, client := net.Pipe()
	t.Cleanup(func() {
		server.Close()
		client.Close()
	})
	c := &Conn{
		conn:      server,
		reader:    bufio.NewReader(server),
		writer:    bufio.NewWriter(server),
		readLimit: defaultReadLimit,
	}
	return c, client
}

func writeMaskedFrame(t *testing.T, w net.Conn, opcode byte, payload []byte) {
	t.Helper()

	b0 := byte(0x80) | opcode
	payloadLen := len(payload)

	var header []byte
	switch {
	case payloadLen < 126:
		header = []byte{b0, byte(payloadLen) | 0x80}
	case payloadLen < 65536:
		header = []byte{b0, 126 | 0x80, 0, 0}
		binary.BigEndian.PutUint16(header[2:], uint16(payloadLen))
	default:
		header = []byte{b0, 127 | 0x80, 0, 0, 0, 0, 0, 0, 0, 0}
		binary.BigEndian.PutUint64(header[2:], uint64(payloadLen))
	}

	maskKey := [4]byte{0x12, 0x34, 0x56, 0x78}
	masked := make([]byte, payloadLen)
	for i, b := range payload {
		masked[i] = b ^ maskKey[i%4]
	}

	frame := append(header, maskKey[:]...)
	frame = append(frame, masked...)
	if _, err := w.Write(frame); err != nil {
		t.Fatalf("write frame: %v", err)
	}
}

type readResult struct {
	mt   MessageType
	data []byte
	err  error
}

func readMessageWithTimeout(t *testing.T, c *Conn) readResult {
	t.Helper()
	resultCh := make(chan readResult, 1)
	go func() {
		mt, data, err := c.ReadMessage()
		resultCh <- readResult{mt, data, err}
	}()

	select {
	case res := <-resultCh:
		return res
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for ReadMessage")
		return readResult{}
	}
}

func TestConn_SetPongHandler(t *testing.T) {
	c, client := newTestConn(t)

	var mu sync.Mutex
	var received [][]byte
	c.SetPongHandler(func(data []byte) {
		mu.Lock()
		defer mu.Unlock()
		got := make([]byte, len(data))
		copy(got, data)
		received = append(received, got)
	})

	writeDone := make(chan struct{})
	go func() {
		defer close(writeDone)
		writeMaskedFrame(t, client, OpPong, []byte("beat"))
		writeMaskedFrame(t, client, OpText, []byte("hello"))
	}()

	res := readMessageWithTimeout(t, c)
	if res.err != nil {
		t.Fatalf("ReadMessage error: %v", res.err)
	}
	if res.mt != TextMessage {
		t.Fatalf("message type: got %v want %v", res.mt, TextMessage)
	}
	if string(res.data) != "hello" {
		t.Fatalf("message data: got %q want %q", res.data, "hello")
	}

	select {
	case <-writeDone:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for writer goroutine")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 1 {
		t.Fatalf("pong handler called %d times, want 1", len(received))
	}
	if string(received[0]) != "beat" {
		t.Fatalf("pong payload: got %q want %q", received[0], "beat")
	}
}

func TestConn_ReadMessage_PongWithoutHandlerIsSwallowed(t *testing.T) {
	c, client := newTestConn(t)

	writeDone := make(chan struct{})
	go func() {
		defer close(writeDone)
		writeMaskedFrame(t, client, OpPong, []byte("beat"))
		writeMaskedFrame(t, client, OpText, []byte("hello"))
	}()

	res := readMessageWithTimeout(t, c)
	if res.err != nil {
		t.Fatalf("ReadMessage error: %v", res.err)
	}
	if res.mt != TextMessage {
		t.Fatalf("message type: got %v want %v", res.mt, TextMessage)
	}
	if string(res.data) != "hello" {
		t.Fatalf("message data: got %q want %q", res.data, "hello")
	}

	select {
	case <-writeDone:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for writer goroutine")
	}
}
