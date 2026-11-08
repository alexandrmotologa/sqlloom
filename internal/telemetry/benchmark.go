package telemetry

import (
	"encoding/binary"
	"fmt"
	"math/rand"
	"net"
	"sort"
	"sync"
	"time"

	"github.com/alexandrmotologa/sqlloom/internal/wire"
)

// BenchmarkParams configures a smoke benchmark run.
type BenchmarkParams struct {
	TotalQueries int     `json:"total_queries"`
	Concurrency  int     `json:"concurrency"`
	WriteRatio   float64 `json:"write_ratio"` // e.g. 0.1 = 10% writes, 90% reads
	Database     string  `json:"database,omitempty"`
	User         string  `json:"user,omitempty"`
	Table        string  `json:"table,omitempty"`
}

// BenchmarkResult holds latency and throughput statistics.
type BenchmarkResult struct {
	TotalQueries int     `json:"total_queries"`
	DurationMs   float64 `json:"duration_ms"`
	ThroughputQPS float64 `json:"throughput_qps"`
	CacheHits    int     `json:"cache_hits"`
	CacheMisses  int     `json:"cache_misses"`
	HitRatioPct  float64 `json:"hit_ratio_pct"`
	P50LatencyMs float64 `json:"p50_latency_ms"`
	P99LatencyMs float64 `json:"p99_latency_ms"`
	MinLatencyMs float64 `json:"min_latency_ms"`
	MaxLatencyMs float64 `json:"max_latency_ms"`
}

// RunSmokeBenchmark executes a concurrent benchmark against the proxy listen address.
func RunSmokeBenchmark(proxyAddr string, params BenchmarkParams) (*BenchmarkResult, error) {
	if params.TotalQueries <= 0 {
		params.TotalQueries = 300
	}
	if params.TotalQueries > 2000 {
		params.TotalQueries = 2000
	}
	if params.Concurrency <= 0 {
		params.Concurrency = 4
	}
	if params.Concurrency > 20 {
		params.Concurrency = 20
	}
	if params.WriteRatio < 0 || params.WriteRatio > 0.5 {
		params.WriteRatio = 0.1
	}
	if params.Database == "" {
		params.Database = "postgres"
	}
	if params.User == "" {
		params.User = "postgres"
	}
	targetTable := params.Table
	if targetTable == "" {
		targetTable = "users"
	}

	queriesPerWorker := params.TotalQueries / params.Concurrency

	var (
		wg         sync.WaitGroup
		mu         sync.Mutex
		latencies  []float64
		cacheHits  int
		cacheMiss  int
	)

	startOverall := time.Now()

	for workerID := 0; workerID < params.Concurrency; workerID++ {
		wg.Add(1)
		go func(wid int) {
			defer wg.Done()

			conn, reader, writer, err := dialBenchClient(proxyAddr, params.Database, params.User)
			if err != nil {
				return
			}
			defer conn.Close()

			localRng := rand.New(rand.NewSource(time.Now().UnixNano() + int64(wid)))

			for i := 0; i < queriesPerWorker; i++ {
				var sql string
				isWrite := localRng.Float64() < params.WriteRatio

				if isWrite {
					sql = fmt.Sprintf("UPDATE %s SET active = true WHERE id = %d", targetTable, localRng.Intn(5)+1)
				} else {
					sql = fmt.Sprintf("SELECT * FROM %s WHERE id = %d", targetTable, localRng.Intn(10)+1)
				}

				t0 := time.Now()
				qMsg := wire.BuildQueryMessage(sql)
				if err := writer.WriteRaw(qMsg); err != nil {
					return
				}
				if err := writer.Flush(); err != nil {
					return
				}

				// Read until ReadyForQuery
				for {
					m, err := reader.ReadMessage()
					if err != nil {
						return
					}
					if m.Type == wire.MsgReadyForQuery {
						break
					}
				}

				latMs := float64(time.Since(t0).Microseconds()) / 1000.0

				mu.Lock()
				latencies = append(latencies, latMs)
				if !isWrite {
					if latMs < 1.0 { // Sub-millisecond indicates cache hit
						cacheHits++
					} else {
						cacheMiss++
					}
				}
				mu.Unlock()
			}
		}(workerID)
	}

	wg.Wait()
	duration := time.Since(startOverall)
	totalRan := len(latencies)
	if totalRan == 0 {
		return nil, fmt.Errorf("benchmark produced no query results")
	}

	sort.Float64s(latencies)

	p50 := latencies[int(float64(totalRan)*0.50)]
	p99 := latencies[int(float64(totalRan)*0.99)]
	minLat := latencies[0]
	maxLat := latencies[totalRan-1]

	durMs := float64(duration.Microseconds()) / 1000.0
	qps := float64(totalRan) / duration.Seconds()

	hitRatio := 0.0
	if cacheHits+cacheMiss > 0 {
		hitRatio = (float64(cacheHits) / float64(cacheHits+cacheMiss)) * 100.0
	}

	return &BenchmarkResult{
		TotalQueries:  totalRan,
		DurationMs:    durMs,
		ThroughputQPS: qps,
		CacheHits:     cacheHits,
		CacheMisses:   cacheMiss,
		HitRatioPct:   hitRatio,
		P50LatencyMs:  p50,
		P99LatencyMs:  p99,
		MinLatencyMs:  minLat,
		MaxLatencyMs:  maxLat,
	}, nil
}

func dialBenchClient(proxyAddr, database, user string) (net.Conn, *wire.Reader, *wire.Writer, error) {
	conn, err := net.DialTimeout("tcp", proxyAddr, 2*time.Second)
	if err != nil {
		return nil, nil, nil, err
	}

	reader := wire.NewReader(conn)
	writer := wire.NewWriter(conn)

	// SSLRequest
	var sslBuf [8]byte
	binary.BigEndian.PutUint32(sslBuf[0:4], 8)
	binary.BigEndian.PutUint32(sslBuf[4:8], uint32(wire.SSLRequestCode))
	if _, err := conn.Write(sslBuf[:]); err != nil {
		_ = conn.Close()
		return nil, nil, nil, err
	}

	var sslReply [1]byte
	if _, err := conn.Read(sslReply[:]); err != nil || sslReply[0] != 'N' {
		_ = conn.Close()
		return nil, nil, nil, fmt.Errorf("unexpected SSL reply")
	}

	// StartupMessage
	var startupPayload []byte
	var proto [4]byte
	binary.BigEndian.PutUint32(proto[:], uint32(wire.ProtocolVersion30))
	startupPayload = append(startupPayload, proto[:]...)
	startupPayload = append(startupPayload, []byte("user\x00"+user+"\x00database\x00"+database+"\x00\x00")...)

	var startupLen [4]byte
	binary.BigEndian.PutUint32(startupLen[:], uint32(len(startupPayload)+4))
	_, _ = conn.Write(startupLen[:])
	_, _ = conn.Write(startupPayload)

	for {
		msg, err := reader.ReadMessage()
		if err != nil {
			_ = conn.Close()
			return nil, nil, nil, err
		}
		if msg.Type == wire.MsgReadyForQuery {
			break
		}
	}

	return conn, reader, writer, nil
}
