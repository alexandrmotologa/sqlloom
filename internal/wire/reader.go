package wire

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"sync"
)

const maxMessageLength = 64 * 1024 * 1024 // 64 MB security ceiling

var headerPool = sync.Pool{
	New: func() any {
		b := make([]byte, 5)
		return &b
	},
}

// Reader decodes PostgreSQL protocol messages from an underlying io.Reader.
type Reader struct {
	r *bufio.Reader
}

// NewReader wraps an io.Reader in a buffered wire protocol reader.
func NewReader(r io.Reader) *Reader {
	return &Reader{
		r: bufio.NewReaderSize(r, 32*1024),
	}
}

// ReadStartup reads the client's initial startup or SSL negotiation packet.
func (r *Reader) ReadStartup() (RawMessage, error) {
	var lenBuf [4]byte
	if _, err := io.ReadFull(r.r, lenBuf[:]); err != nil {
		return RawMessage{}, err
	}

	length := int32(binary.BigEndian.Uint32(lenBuf[:]))
	if length < 4 || length > maxMessageLength {
		return RawMessage{}, fmt.Errorf("invalid startup message length: %d", length)
	}

	payloadLen := int(length - 4)
	payload := make([]byte, payloadLen)
	if payloadLen > 0 {
		if _, err := io.ReadFull(r.r, payload); err != nil {
			return RawMessage{}, err
		}
	}

	return RawMessage{
		Type:    0,
		Payload: payload,
	}, nil
}

// ReadMessage reads a standard PostgreSQL framed message [Type: 1 byte][Length: 4 bytes][Payload].
func (r *Reader) ReadMessage() (RawMessage, error) {
	headerPtr := headerPool.Get().(*[]byte)
	header := *headerPtr
	defer headerPool.Put(headerPtr)

	if _, err := io.ReadFull(r.r, header); err != nil {
		return RawMessage{}, err
	}

	msgType := header[0]
	msgLen := int32(binary.BigEndian.Uint32(header[1:5]))

	if msgLen < 4 || msgLen > maxMessageLength {
		return RawMessage{}, fmt.Errorf("invalid message length for type '%c' (0x%02x): %d", msgType, msgType, msgLen)
	}

	payloadLen := int(msgLen - 4)
	var payload []byte
	if payloadLen > 0 {
		payload = make([]byte, payloadLen)
		if _, err := io.ReadFull(r.r, payload); err != nil {
			return RawMessage{}, err
		}
	}

	return RawMessage{
		Type:    msgType,
		Payload: payload,
	}, nil
}
