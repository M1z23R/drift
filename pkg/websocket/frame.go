package websocket

import (
	"bufio"
	"encoding/binary"
	"errors"
	"io"
)

// Opcodes defined by RFC 6455
const (
	OpContinuation = 0x0
	OpText         = 0x1
	OpBinary       = 0x2
	OpClose        = 0x8
	OpPing         = 0x9
	OpPong         = 0xA
)

// Close codes defined by RFC 6455
const (
	CloseNormalClosure           = 1000
	CloseGoingAway               = 1001
	CloseProtocolError           = 1002
	CloseUnsupportedData         = 1003
	CloseNoStatusReceived        = 1005
	CloseAbnormalClosure         = 1006
	CloseInvalidFramePayloadData = 1007
	ClosePolicyViolation         = 1008
	CloseMessageTooBig           = 1009
	CloseMandatoryExtension      = 1010
	CloseInternalServerErr       = 1011
	CloseTLSHandshake            = 1015
)

// Frame size limits
const (
	maxFrameHeaderSize = 14
	maxControlPayload  = 125
	defaultReadLimit   = 32 * 1024 * 1024 // 32MB
)

var (
	ErrFrameTooLarge    = errors.New("websocket: frame too large")
	ErrInvalidOpcode    = errors.New("websocket: invalid opcode")
	ErrReservedBits     = errors.New("websocket: reserved bits set")
	ErrControlFragment  = errors.New("websocket: control frame cannot be fragmented")
	ErrControlTooLarge  = errors.New("websocket: control frame payload too large")
	ErrUnmaskedFrame    = errors.New("websocket: client frame not masked")
	ErrMaskedFrame      = errors.New("websocket: server frame masked")
	ErrContinuation     = errors.New("websocket: unexpected continuation frame")
	ErrNotContinuation  = errors.New("websocket: expected continuation frame")
	ErrConnectionClosed = errors.New("websocket: connection closed")
)

// frame represents a WebSocket frame
type frame struct {
	fin     bool
	rsv1    bool
	rsv2    bool
	rsv3    bool
	opcode  byte
	masked  bool
	maskKey [4]byte
	payload []byte
}

// readFrame reads a WebSocket frame from the connection
func readFrame(r *bufio.Reader, maxSize int64) (*frame, error) {
	// Read first 2 bytes
	header := make([]byte, 2)
	if _, err := io.ReadFull(r, header); err != nil {
		return nil, err
	}

	f := &frame{}

	// Parse first byte: FIN, RSV1-3, Opcode
	f.fin = header[0]&0x80 != 0
	f.rsv1 = header[0]&0x40 != 0
	f.rsv2 = header[0]&0x20 != 0
	f.rsv3 = header[0]&0x10 != 0
	f.opcode = header[0] & 0x0F

	// Check reserved bits (must be 0 unless extensions are negotiated)
	if f.rsv1 || f.rsv2 || f.rsv3 {
		return nil, ErrReservedBits
	}

	// Parse second byte: MASK, Payload length
	f.masked = header[1]&0x80 != 0
	payloadLen := uint64(header[1] & 0x7F)

	// Extended payload length
	switch payloadLen {
	case 126:
		// 16-bit length
		lenBytes := make([]byte, 2)
		if _, err := io.ReadFull(r, lenBytes); err != nil {
			return nil, err
		}
		payloadLen = uint64(binary.BigEndian.Uint16(lenBytes))
	case 127:
		// 64-bit length
		lenBytes := make([]byte, 8)
		if _, err := io.ReadFull(r, lenBytes); err != nil {
			return nil, err
		}
		payloadLen = binary.BigEndian.Uint64(lenBytes)
	}

	// Check payload size
	if maxSize > 0 && int64(payloadLen) > maxSize {
		return nil, ErrFrameTooLarge
	}

	// Validate control frames
	if f.opcode >= OpClose {
		if !f.fin {
			return nil, ErrControlFragment
		}
		if payloadLen > maxControlPayload {
			return nil, ErrControlTooLarge
		}
	}

	// Read masking key if present
	if f.masked {
		if _, err := io.ReadFull(r, f.maskKey[:]); err != nil {
			return nil, err
		}
	}

	// Read payload
	if payloadLen > 0 {
		f.payload = make([]byte, payloadLen)
		if _, err := io.ReadFull(r, f.payload); err != nil {
			return nil, err
		}

		// Unmask if needed
		if f.masked {
			maskBytes(f.maskKey, f.payload)
		}
	}

	return f, nil
}

// writeFrame writes a WebSocket frame to the connection
func writeFrame(w *bufio.Writer, opcode byte, payload []byte, fin bool) error {
	// First byte: FIN + Opcode
	b0 := opcode
	if fin {
		b0 |= 0x80
	}

	payloadLen := len(payload)

	// Calculate header size and write header
	if payloadLen < 126 {
		// 2-byte header
		if _, err := w.Write([]byte{b0, byte(payloadLen)}); err != nil {
			return err
		}
	} else if payloadLen < 65536 {
		// 4-byte header
		header := []byte{b0, 126, 0, 0}
		binary.BigEndian.PutUint16(header[2:], uint16(payloadLen))
		if _, err := w.Write(header); err != nil {
			return err
		}
	} else {
		// 10-byte header
		header := []byte{b0, 127, 0, 0, 0, 0, 0, 0, 0, 0}
		binary.BigEndian.PutUint64(header[2:], uint64(payloadLen))
		if _, err := w.Write(header); err != nil {
			return err
		}
	}

	// Write payload
	if payloadLen > 0 {
		if _, err := w.Write(payload); err != nil {
			return err
		}
	}

	return w.Flush()
}

// maskBytes applies XOR mask to data in place
func maskBytes(key [4]byte, data []byte) {
	for i := range data {
		data[i] ^= key[i%4]
	}
}
