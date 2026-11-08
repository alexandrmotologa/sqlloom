package wire

import (
	"encoding/binary"
	"fmt"
	"net"
)

// HandshakeResult contains details collected during the initial connection handshake.
type HandshakeResult struct {
	StartupMsg  *StartupMessage
	Database    string
	User        string
	BackendPID  uint32
	BackendKey  uint32
	TxStatus    byte
	Params      map[string]string
}

// PerformClientHandshake handles initial SSL / GSS negotiation and reads the StartupMessage.
func PerformClientHandshake(clientConn net.Conn, clientReader *Reader) (*StartupMessage, error) {
	for {
		msg, err := clientReader.ReadStartup()
		if err != nil {
			return nil, fmt.Errorf("failed to read startup packet: %w", err)
		}

		if len(msg.Payload) < 4 {
			return nil, fmt.Errorf("startup packet too short: %d", len(msg.Payload))
		}

		proto := int32(binary.BigEndian.Uint32(msg.Payload[0:4]))

		if proto == SSLRequestCode || proto == GSSENCRequestCode {
			// Proxy instructs client to continue in plain mode
			if _, err := clientConn.Write([]byte{'N'}); err != nil {
				return nil, fmt.Errorf("failed to respond to SSL/GSS request: %w", err)
			}
			continue
		}

		// Standard StartupMessage
		startup, err := ParseStartupMessage(msg.Payload)
		if err != nil {
			return nil, fmt.Errorf("failed to parse startup message: %w", err)
		}

		return startup, nil
	}
}

// ForwardHandshakeToUpstream passes the startup packet and completes authentication with upstream PG.
func ForwardHandshakeToUpstream(
	startup *StartupMessage,
	clientReader *Reader,
	clientWriter *Writer,
	upstreamConn net.Conn,
	upstreamReader *Reader,
	upstreamWriter *Writer,
) (*HandshakeResult, error) {
	// Reconstruct and send startup packet to upstream
	var payload []byte
	var protoBuf [4]byte
	binary.BigEndian.PutUint32(protoBuf[:], uint32(startup.ProtocolVersion))
	payload = append(payload, protoBuf[:]...)

	for k, v := range startup.Parameters {
		payload = append(payload, k...)
		payload = append(payload, 0)
		payload = append(payload, v...)
		payload = append(payload, 0)
	}
	payload = append(payload, 0) // Final null terminator

	if err := upstreamWriter.WriteMessage(0, payload); err != nil {
		return nil, fmt.Errorf("failed to send startup to upstream: %w", err)
	}
	if err := upstreamWriter.Flush(); err != nil {
		return nil, fmt.Errorf("failed to flush startup to upstream: %w", err)
	}

	result := &HandshakeResult{
		StartupMsg: startup,
		Database:   startup.Parameters["database"],
		User:       startup.Parameters["user"],
		Params:     make(map[string]string),
		TxStatus:   TxStatusIdle,
	}

	// Relay authentication and configuration messages until ReadyForQuery ('Z')
	for {
		msg, err := upstreamReader.ReadMessage()
		if err != nil {
			return nil, fmt.Errorf("failed reading from upstream during handshake: %w", err)
		}

		switch msg.Type {
		case MsgAuth:
			if err := clientWriter.WriteRaw(msg); err != nil {
				return nil, err
			}
			if err := clientWriter.Flush(); err != nil {
				return nil, err
			}

			if len(msg.Payload) < 4 {
				return nil, fmt.Errorf("invalid auth message payload")
			}
			authType := binary.BigEndian.Uint32(msg.Payload[0:4])
			if authType == 0 {
				// AuthenticationOk, continue reading parameter statuses
				continue
			}

			// Password or SASL challenge: read client response and forward to upstream
			clientResp, err := clientReader.ReadMessage()
			if err != nil {
				return nil, fmt.Errorf("failed reading auth response from client: %w", err)
			}
			if err := upstreamWriter.WriteRaw(clientResp); err != nil {
				return nil, fmt.Errorf("failed forwarding auth response to upstream: %w", err)
			}
			if err := upstreamWriter.Flush(); err != nil {
				return nil, fmt.Errorf("failed flushing auth response to upstream: %w", err)
			}

		case MsgParameterStatus:
			// Capture parameter status (server_version, client_encoding, etc.)
			payload := msg.Payload
			if idx := bytesIndex(payload, 0); idx != -1 {
				key := string(payload[:idx])
				rest := payload[idx+1:]
				if valIdx := bytesIndex(rest, 0); valIdx != -1 {
					result.Params[key] = string(rest[:valIdx])
				}
			}
			if err := clientWriter.WriteRaw(msg); err != nil {
				return nil, err
			}

		case MsgBackendKeyData:
			if len(msg.Payload) >= 8 {
				result.BackendPID = binary.BigEndian.Uint32(msg.Payload[0:4])
				result.BackendKey = binary.BigEndian.Uint32(msg.Payload[4:8])
			}
			if err := clientWriter.WriteRaw(msg); err != nil {
				return nil, err
			}

		case MsgErrorResponse:
			_ = clientWriter.WriteRaw(msg)
			_ = clientWriter.Flush()
			return nil, fmt.Errorf("upstream rejected connection with ErrorResponse")

		case MsgReadyForQuery:
			if len(msg.Payload) > 0 {
				result.TxStatus = msg.Payload[0]
			}
			if err := clientWriter.WriteRaw(msg); err != nil {
				return nil, err
			}
			if err := clientWriter.Flush(); err != nil {
				return nil, err
			}
			// Handshake complete!
			return result, nil

		default:
			if err := clientWriter.WriteRaw(msg); err != nil {
				return nil, err
			}
		}
	}
}

func bytesIndex(data []byte, b byte) int {
	for i, v := range data {
		if v == b {
			return i
		}
	}
	return -1
}
