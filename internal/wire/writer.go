package wire

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
)

// Writer serializes PostgreSQL wire protocol packets to an io.Writer.
type Writer struct {
	w *bufio.Writer
}

// NewWriter creates a buffered wire protocol writer.
func NewWriter(w io.Writer) *Writer {
	return &Writer{
		w: bufio.NewWriterSize(w, 32*1024),
	}
}

// WriteMessage writes a framed message [Type][Length(4)][Payload] to the buffer.
func (w *Writer) WriteMessage(msgType byte, payload []byte) error {
	if msgType == 0 {
		// Startup message (length header includes itself)
		var lenBuf [4]byte
		binary.BigEndian.PutUint32(lenBuf[:], uint32(len(payload)+4))
		if _, err := w.w.Write(lenBuf[:]); err != nil {
			return err
		}
		if len(payload) > 0 {
			if _, err := w.w.Write(payload); err != nil {
				return err
			}
		}
		return nil
	}

	if err := w.w.WriteByte(msgType); err != nil {
		return err
	}

	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(len(payload)+4))
	if _, err := w.w.Write(lenBuf[:]); err != nil {
		return err
	}

	if len(payload) > 0 {
		if _, err := w.w.Write(payload); err != nil {
			return err
		}
	}

	return nil
}

// WriteRaw serializes and flushes a RawMessage.
func (w *Writer) WriteRaw(m RawMessage) error {
	return w.WriteMessage(m.Type, m.Payload)
}

// WriteBytes writes raw byte buffer directly.
func (w *Writer) WriteBytes(b []byte) error {
	_, err := w.w.Write(b)
	return err
}

// Flush ensures all buffered data is written to the underlying connection.
func (w *Writer) Flush() error {
	return w.w.Flush()
}

// WriteError sends a standard PostgreSQL ErrorResponse ('E') message.
func (w *Writer) WriteError(severity, code, message string) error {
	var buf []byte
	buf = append(buf, 'S')
	buf = append(buf, severity...)
	buf = append(buf, 0)

	buf = append(buf, 'C')
	buf = append(buf, code...)
	buf = append(buf, 0)

	buf = append(buf, 'M')
	buf = append(buf, message...)
	buf = append(buf, 0)

	buf = append(buf, 0) // Final null terminator

	if err := w.WriteMessage(MsgErrorResponse, buf); err != nil {
		return fmt.Errorf("failed to write error response: %w", err)
	}
	return w.Flush()
}
