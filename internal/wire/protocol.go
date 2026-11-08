package wire

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// PostgreSQL 3.0 protocol message type identifiers.
const (
	// Frontend message types
	MsgQuery     byte = 'Q'
	MsgParse     byte = 'P'
	MsgBind      byte = 'B'
	MsgDescribe  byte = 'D'
	MsgExecute   byte = 'E'
	MsgSync      byte = 'S'
	MsgFlush     byte = 'H'
	MsgClose     byte = 'C'
	MsgPassword  byte = 'p'
	MsgTerminate byte = 'X'
	MsgCopyData  byte = 'd'
	MsgCopyDone  byte = 'c'
	MsgCopyFail  byte = 'f'

	// Backend message types
	MsgAuth               byte = 'R'
	MsgBackendKeyData     byte = 'K'
	MsgParameterStatus    byte = 'S'
	MsgReadyForQuery      byte = 'Z'
	MsgRowDescription     byte = 'T'
	MsgDataRow            byte = 'D'
	MsgCommandComplete    byte = 'C'
	MsgErrorResponse      byte = 'E'
	MsgNoticeResponse     byte = 'N'
	MsgParseComplete      byte = '1'
	MsgBindComplete       byte = '2'
	MsgCloseComplete      byte = '3'
	MsgNoData             byte = 'n'
	MsgPortalSuspended    byte = 's'
	MsgEmptyQueryResponse byte = 'I'
	MsgParamDescription   byte = 't'

	// Special startup protocol codes
	SSLRequestCode    int32 = 80877103
	CancelRequestCode int32 = 80877102
	GSSENCRequestCode int32 = 80877104
	ProtocolVersion30 int32 = 196608 // 0x00030000

	// ReadyForQuery transaction statuses
	TxStatusIdle   byte = 'I' // Idle, not in a transaction block
	TxStatusInTx   byte = 'T' // In an active transaction block
	TxStatusFailed byte = 'E' // In a failed transaction block
)

// RawMessage represents a framed PostgreSQL protocol message.
// For standard messages, Type is the leading byte and Payload is the body (excluding the 4-byte length header).
// For startup messages, Type is 0.
type RawMessage struct {
	Type    byte
	Payload []byte
}

// Clone returns an independent deep copy of the message.
func (m RawMessage) Clone() RawMessage {
	c := RawMessage{
		Type:    m.Type,
		Payload: make([]byte, len(m.Payload)),
	}
	copy(c.Payload, m.Payload)
	return c
}

// Encode serializes the message into wire format [Type][Length(4)][Payload].
func (m RawMessage) Encode() []byte {
	if m.Type == 0 {
		// Startup packet: Length (4 bytes) + Payload
		length := int32(len(m.Payload) + 4)
		buf := make([]byte, 4+len(m.Payload))
		binary.BigEndian.PutUint32(buf[0:4], uint32(length))
		copy(buf[4:], m.Payload)
		return buf
	}

	length := int32(len(m.Payload) + 4)
	buf := make([]byte, 1+4+len(m.Payload))
	buf[0] = m.Type
	binary.BigEndian.PutUint32(buf[1:5], uint32(length))
	copy(buf[5:], m.Payload)
	return buf
}

// StartupMessage represents a parsed client startup message.
type StartupMessage struct {
	ProtocolVersion int32
	Parameters      map[string]string
}

// ParseStartupMessage extracts startup parameters from a startup packet payload.
func ParseStartupMessage(payload []byte) (*StartupMessage, error) {
	if len(payload) < 4 {
		return nil, fmt.Errorf("startup payload too short: %d bytes", len(payload))
	}

	proto := int32(binary.BigEndian.Uint32(payload[0:4]))
	params := make(map[string]string)

	rest := payload[4:]
	for len(rest) > 0 {
		idx := bytes.IndexByte(rest, 0)
		if idx == -1 || idx == 0 {
			break
		}
		key := string(rest[:idx])
		rest = rest[idx+1:]

		valIdx := bytes.IndexByte(rest, 0)
		if valIdx == -1 {
			break
		}
		val := string(rest[:valIdx])
		rest = rest[valIdx+1:]

		params[key] = val
	}

	return &StartupMessage{
		ProtocolVersion: proto,
		Parameters:      params,
	}, nil
}

// ParseQueryMessage extracts the raw SQL string from a 'Q' message payload.
func ParseQueryMessage(payload []byte) (string, error) {
	idx := bytes.IndexByte(payload, 0)
	if idx == -1 {
		return string(payload), nil
	}
	return string(payload[:idx]), nil
}

// BuildQueryMessage creates a standard 'Q' query packet.
func BuildQueryMessage(sql string) RawMessage {
	payload := make([]byte, len(sql)+1)
	copy(payload, sql)
	payload[len(sql)] = 0
	return RawMessage{
		Type:    MsgQuery,
		Payload: payload,
	}
}

// BuildReadyForQuery creates a 'Z' message with transaction status.
func BuildReadyForQuery(txStatus byte) RawMessage {
	return RawMessage{
		Type:    MsgReadyForQuery,
		Payload: []byte{txStatus},
	}
}

// FieldDescription holds metadata for a single column in RowDescription ('T').
type FieldDescription struct {
	Name         string
	TableOID     uint32
	ColumnAttr   int16
	DataTypeOID  uint32
	DataTypeSize int16
	TypeModifier int32
	FormatCode   int16 // 0 = text, 1 = binary
}

// ParseRowDescription decodes a 'T' message into column metadata.
func ParseRowDescription(payload []byte) ([]FieldDescription, error) {
	if len(payload) < 2 {
		return nil, fmt.Errorf("row description payload too short: %d", len(payload))
	}

	numFields := int(binary.BigEndian.Uint16(payload[0:2]))
	fields := make([]FieldDescription, 0, numFields)
	offset := 2

	for i := 0; i < numFields; i++ {
		idx := bytes.IndexByte(payload[offset:], 0)
		if idx == -1 {
			return nil, fmt.Errorf("unterminated field name at field %d", i)
		}
		name := string(payload[offset : offset+idx])
		offset += idx + 1

		if len(payload)-offset < 18 {
			return nil, fmt.Errorf("truncated field description metadata for field %s", name)
		}

		tableOID := binary.BigEndian.Uint32(payload[offset : offset+4])
		colAttr := int16(binary.BigEndian.Uint16(payload[offset+4 : offset+6]))
		typeOID := binary.BigEndian.Uint32(payload[offset+6 : offset+10])
		typeSize := int16(binary.BigEndian.Uint16(payload[offset+10 : offset+12]))
		typeMod := int32(binary.BigEndian.Uint32(payload[offset+12 : offset+16]))
		format := int16(binary.BigEndian.Uint16(payload[offset+16 : offset+18]))
		offset += 18

		fields = append(fields, FieldDescription{
			Name:         name,
			TableOID:     tableOID,
			ColumnAttr:   colAttr,
			DataTypeOID:  typeOID,
			DataTypeSize: typeSize,
			TypeModifier: typeMod,
			FormatCode:   format,
		})
	}

	return fields, nil
}
