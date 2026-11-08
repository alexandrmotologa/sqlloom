package test

import (
	"bytes"
	"encoding/binary"
	"net"
	"sync"
	"sync/atomic"

	"github.com/alexandrmotologa/sqlloom/internal/wire"
)

// MockPostgresServer simulates an upstream PostgreSQL server for deterministic testing.
type MockPostgresServer struct {
	Listener      net.Listener
	Addr          string
	QueryCount    atomic.Int64
	LastQuery     string
	mu            sync.Mutex
	closed        chan struct{}
}

// StartMockPostgres initializes and starts listening on an ephemeral port.
func StartMockPostgres() (*MockPostgresServer, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}

	mock := &MockPostgresServer{
		Listener: ln,
		Addr:     ln.Addr().String(),
		closed:   make(chan struct{}),
	}

	go mock.acceptLoop()
	return mock, nil
}

func (m *MockPostgresServer) acceptLoop() {
	for {
		conn, err := m.Listener.Accept()
		if err != nil {
			select {
			case <-m.closed:
				return
			default:
				return
			}
		}
		go m.handleConn(conn)
	}
}

func (m *MockPostgresServer) handleConn(conn net.Conn) {
	defer conn.Close()

	reader := wire.NewReader(conn)
	writer := wire.NewWriter(conn)

	// 1. Handshake
	for {
		msg, err := reader.ReadStartup()
		if err != nil {
			return
		}

		if len(msg.Payload) >= 4 {
			proto := int32(binary.BigEndian.Uint32(msg.Payload[0:4]))
			if proto == wire.SSLRequestCode || proto == wire.GSSENCRequestCode {
				_, _ = conn.Write([]byte{'N'})
				continue
			}
		}
		break
	}

	// Send AuthenticationOk ('R', 0)
	var authOk [4]byte
	binary.BigEndian.PutUint32(authOk[:], 0)
	_ = writer.WriteMessage(wire.MsgAuth, authOk[:])

	// Send ParameterStatus
	_ = writer.WriteMessage(wire.MsgParameterStatus, []byte("server_version\x0016.1\x00"))
	_ = writer.WriteMessage(wire.MsgParameterStatus, []byte("client_encoding\x00UTF8\x00"))

	// Send BackendKeyData ('K')
	var keyData [8]byte
	binary.BigEndian.PutUint32(keyData[0:4], 12345)
	binary.BigEndian.PutUint32(keyData[4:8], 67890)
	_ = writer.WriteMessage(wire.MsgBackendKeyData, keyData[:])

	// ReadyForQuery ('Z', 'I')
	_ = writer.WriteMessage(wire.MsgReadyForQuery, []byte{wire.TxStatusIdle})
	_ = writer.Flush()

	txStatus := wire.TxStatusIdle

	// 2. Query processing loop
	for {
		msg, err := reader.ReadMessage()
		if err != nil {
			return
		}

		if msg.Type == wire.MsgQuery {
			rawSQL, _ := wire.ParseQueryMessage(msg.Payload)
			m.QueryCount.Add(1)

			m.mu.Lock()
			m.LastQuery = rawSQL
			m.mu.Unlock()

			if bytes.HasPrefix([]byte(rawSQL), []byte("SELECT")) {
				// Send RowDescription for 2 columns: id (int4), name (text)
				var rowDesc []byte
				// numFields = 2
				var numFields [2]byte
				binary.BigEndian.PutUint16(numFields[:], 2)
				rowDesc = append(rowDesc, numFields[:]...)

				// Column 1: "id"
				rowDesc = append(rowDesc, []byte("id\x00")...)
				var meta1 [18]byte
				binary.BigEndian.PutUint32(meta1[0:4], 0)      // Table OID
				binary.BigEndian.PutUint16(meta1[4:6], 1)      // Col attr
				binary.BigEndian.PutUint32(meta1[6:10], 23)    // Type OID (int4)
				binary.BigEndian.PutUint16(meta1[10:12], 4)    // Type size
				binary.BigEndian.PutUint32(meta1[12:16], 0xFFFFFFFF) // Type mod (-1)
				binary.BigEndian.PutUint16(meta1[16:18], 0)    // Format code
				rowDesc = append(rowDesc, meta1[:]...)

				// Column 2: "name"
				rowDesc = append(rowDesc, []byte("name\x00")...)
				var meta2 [18]byte
				binary.BigEndian.PutUint32(meta2[0:4], 0)
				binary.BigEndian.PutUint16(meta2[4:6], 2)
				binary.BigEndian.PutUint32(meta2[6:10], 25) // Type OID (text)
				binary.BigEndian.PutUint16(meta2[10:12], 0xFFFF) // Type size (-1)
				binary.BigEndian.PutUint32(meta2[12:16], 0xFFFFFFFF) // Type mod (-1)
				binary.BigEndian.PutUint16(meta2[16:18], 0)
				rowDesc = append(rowDesc, meta2[:]...)

				_ = writer.WriteMessage(wire.MsgRowDescription, rowDesc)

				// Send DataRow ('D'): col 1 = "42", col 2 = "Alice"
				var dataRow []byte
				var numCols [2]byte
				binary.BigEndian.PutUint16(numCols[:], 2)
				dataRow = append(dataRow, numCols[:]...)

				// col 1: len 2, "42"
				var col1Len [4]byte
				binary.BigEndian.PutUint32(col1Len[:], 2)
				dataRow = append(dataRow, col1Len[:]...)
				dataRow = append(dataRow, []byte("42")...)

				// col 2: len 5, "Alice"
				var col2Len [4]byte
				binary.BigEndian.PutUint32(col2Len[:], 5)
				dataRow = append(dataRow, col2Len[:]...)
				dataRow = append(dataRow, []byte("Alice")...)

				_ = writer.WriteMessage(wire.MsgDataRow, dataRow)
				_ = writer.WriteMessage(wire.MsgCommandComplete, []byte("SELECT 1\x00"))
				_ = writer.WriteMessage(wire.MsgReadyForQuery, []byte{txStatus})
				_ = writer.Flush()

			} else if bytes.HasPrefix([]byte(rawSQL), []byte("UPDATE")) {
				_ = writer.WriteMessage(wire.MsgCommandComplete, []byte("UPDATE 1\x00"))
				_ = writer.WriteMessage(wire.MsgReadyForQuery, []byte{txStatus})
				_ = writer.Flush()

			} else if bytes.HasPrefix([]byte(rawSQL), []byte("INSERT")) {
				_ = writer.WriteMessage(wire.MsgCommandComplete, []byte("INSERT 0 1\x00"))
				_ = writer.WriteMessage(wire.MsgReadyForQuery, []byte{txStatus})
				_ = writer.Flush()

			} else if bytes.HasPrefix([]byte(rawSQL), []byte("BEGIN")) {
				txStatus = wire.TxStatusInTx
				_ = writer.WriteMessage(wire.MsgCommandComplete, []byte("BEGIN\x00"))
				_ = writer.WriteMessage(wire.MsgReadyForQuery, []byte{txStatus})
				_ = writer.Flush()

			} else if bytes.HasPrefix([]byte(rawSQL), []byte("COMMIT")) {
				txStatus = wire.TxStatusIdle
				_ = writer.WriteMessage(wire.MsgCommandComplete, []byte("COMMIT\x00"))
				_ = writer.WriteMessage(wire.MsgReadyForQuery, []byte{txStatus})
				_ = writer.Flush()

			} else {
				_ = writer.WriteMessage(wire.MsgCommandComplete, []byte("OK\x00"))
				_ = writer.WriteMessage(wire.MsgReadyForQuery, []byte{txStatus})
				_ = writer.Flush()
			}
		}
	}
}

// Close stops the mock server.
func (m *MockPostgresServer) Close() error {
	close(m.closed)
	return m.Listener.Close()
}
