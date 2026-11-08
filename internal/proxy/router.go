package proxy

import (
	"time"

	"github.com/alexandrmotologa/sqlloom/internal/cache"
	"github.com/alexandrmotologa/sqlloom/internal/parser"
	"github.com/alexandrmotologa/sqlloom/internal/telemetry"
	"github.com/alexandrmotologa/sqlloom/internal/wire"
)

// Router coordinates message routing between client and upstream, applying cache intercepts.
type Router struct {
	session *Session
}

// NewRouter constructs a router for a given session.
func NewRouter(session *Session) *Router {
	return &Router{session: session}
}

// Dispatch directs an incoming client message.
func (r *Router) Dispatch(msg wire.RawMessage) error {
	switch msg.Type {
	case wire.MsgQuery:
		return r.handleSimpleQuery(msg)
	case wire.MsgParse:
		return r.handleParse(msg)
	case wire.MsgBind:
		return r.handleBind(msg)
	case wire.MsgDescribe, wire.MsgExecute, wire.MsgSync, wire.MsgFlush, wire.MsgClose:
		return r.handleExtendedPassthrough(msg)
	case wire.MsgTerminate:
		return nil
	default:
		// Forward any unintercepted frontend message directly to upstream
		if err := r.session.upstream.Writer.WriteRaw(msg); err != nil {
			return err
		}
		return r.session.upstream.Writer.Flush()
	}
}

func (r *Router) handleSimpleQuery(msg wire.RawMessage) error {
	rawSQL, err := wire.ParseQueryMessage(msg.Payload)
	if err != nil {
		rawSQL = string(msg.Payload)
	}

	r.session.metrics.TotalQueries.Add(1)
	decision := parser.EvaluateCacheability(rawSQL, r.session.txStatus)

	// Route 1: Cacheable SELECT query
	if decision.Cacheable && !r.session.cfg.DisableCache {
		normSQL, _ := parser.NormalizeQuery(rawSQL)
		cacheKey := parser.ComputeCacheKey(r.session.database, normSQL, nil)

		start := time.Now()
		cachedEntry, found := r.session.cache.Get(cacheKey)

		if found {
			return r.serveCachedEntry(cachedEntry, rawSQL, normSQL, decision.Tables, start)
		}

		// SingleFlight: deduplicate concurrent cache misses across goroutines
		entry, shared, err := r.session.singleflight.Do(cacheKey, func() (*cache.CacheEntry, error) {
			return r.executeAndCacheSimpleQuery(msg, rawSQL, normSQL, cacheKey, decision)
		})

		if err != nil {
			// Grace Mode: serve stale cache if upstream is unreachable or returns error
			if r.session.cfg.GraceMode {
				if stale, ok := r.session.cache.Get(cacheKey); ok {
					return r.serveCachedEntry(stale, rawSQL, normSQL, decision.Tables, start)
				}
			}
			return err
		}

		if shared && entry != nil {
			// Shared execution: serve duplicate caller directly from memory
			return r.serveCachedEntry(entry, rawSQL, normSQL, decision.Tables, start)
		}

		return nil
	}

	// Route 2: Non-cacheable queries (writes, transactions, volatile functions, DDL)
	return r.executePassthroughSimpleQuery(msg, rawSQL, decision)
}

func (r *Router) serveCachedEntry(entry *cache.CacheEntry, rawSQL, normSQL string, tables []string, start time.Time) error {
	if err := r.session.clientWriter.WriteRaw(entry.RowDescription); err != nil {
		return err
	}
	for _, row := range entry.DataRows {
		if err := r.session.clientWriter.WriteRaw(row); err != nil {
			return err
		}
	}
	if err := r.session.clientWriter.WriteRaw(entry.CommandComplete); err != nil {
		return err
	}
	if err := r.session.clientWriter.WriteRaw(wire.BuildReadyForQuery(r.session.txStatus)); err != nil {
		return err
	}
	if err := r.session.clientWriter.Flush(); err != nil {
		return err
	}

	elapsed := time.Since(start)
	elapsedMs := float64(elapsed.Microseconds()) / 1000.0

	r.session.metrics.CacheHits.Add(1)
	// Track latency saved compared to typical 15ms database roundtrip
	r.session.metrics.SavedLatencyMicros.Add(15000 - elapsed.Microseconds())

	r.session.hub.Broadcast(telemetry.QueryEvent{
		SQL:        rawSQL,
		Normalized: normSQL,
		Status:     "HIT",
		DurationMs: elapsedMs,
		Tables:     tables,
	})

	return nil
}

func (r *Router) executeAndCacheSimpleQuery(
	msg wire.RawMessage,
	rawSQL, normSQL, cacheKey string,
	decision parser.CacheDecision,
) (*cache.CacheEntry, error) {
	start := time.Now()

	if err := r.session.upstream.Writer.WriteRaw(msg); err != nil {
		return nil, err
	}
	if err := r.session.upstream.Writer.Flush(); err != nil {
		return nil, err
	}

	var rowDesc wire.RawMessage
	var dataRows []wire.RawMessage
	var commandComplete wire.RawMessage
	var hadError bool

	for {
		resp, err := r.session.upstream.Reader.ReadMessage()
		if err != nil {
			return nil, err
		}

		// Stream packet back to client
		if err := r.session.clientWriter.WriteRaw(resp); err != nil {
			return nil, err
		}

		switch resp.Type {
		case wire.MsgRowDescription:
			rowDesc = resp.Clone()
		case wire.MsgDataRow:
			dataRows = append(dataRows, resp.Clone())
		case wire.MsgCommandComplete:
			commandComplete = resp.Clone()
		case wire.MsgErrorResponse:
			hadError = true
		case wire.MsgReadyForQuery:
			if len(resp.Payload) > 0 {
				r.session.txStatus = resp.Payload[0]
			}
			if err := r.session.clientWriter.Flush(); err != nil {
				return nil, err
			}

			elapsed := time.Since(start)
			elapsedMs := float64(elapsed.Microseconds()) / 1000.0
			r.session.metrics.CacheMisses.Add(1)

			var createdEntry *cache.CacheEntry

			// Store in cache if query completed successfully
			if !hadError && rowDesc.Type == wire.MsgRowDescription {
				ttl := time.Duration(r.session.cfg.DefaultTTLSec) * time.Second
				if decision.CustomTTL > 0 {
					ttl = time.Duration(decision.CustomTTL) * time.Second
				}
				createdEntry = cache.NewCacheEntry(
					cacheKey,
					normSQL,
					decision.Tables,
					rowDesc,
					dataRows,
					commandComplete,
					ttl,
				)
				r.session.invalidator.Register(createdEntry)
			}

			r.session.hub.Broadcast(telemetry.QueryEvent{
				SQL:        rawSQL,
				Normalized: normSQL,
				Status:     "MISS",
				DurationMs: elapsedMs,
				Tables:     decision.Tables,
			})

			return createdEntry, nil
		}
	}
}

func (r *Router) executePassthroughSimpleQuery(
	msg wire.RawMessage,
	rawSQL string,
	decision parser.CacheDecision,
) error {
	start := time.Now()

	if err := r.session.upstream.Writer.WriteRaw(msg); err != nil {
		return err
	}
	if err := r.session.upstream.Writer.Flush(); err != nil {
		return err
	}

	var writeCompleted bool

	for {
		resp, err := r.session.upstream.Reader.ReadMessage()
		if err != nil {
			return err
		}

		if err := r.session.clientWriter.WriteRaw(resp); err != nil {
			return err
		}

		switch resp.Type {
		case wire.MsgCommandComplete:
			switch decision.Kind {
			case parser.StmtInsert, parser.StmtUpdate, parser.StmtDelete, parser.StmtTruncate, parser.StmtDDL:
				writeCompleted = true
			}

		case wire.MsgReadyForQuery:
			newTxStatus := r.session.txStatus
			if len(resp.Payload) > 0 {
				newTxStatus = resp.Payload[0]
			}

			if err := r.session.clientWriter.Flush(); err != nil {
				return err
			}

			elapsed := time.Since(start)
			elapsedMs := float64(elapsed.Microseconds()) / 1000.0

			// Handle Table Invalidation
			if writeCompleted {
				if r.session.txStatus == wire.TxStatusInTx {
					// In transaction: buffer modified tables until COMMIT
					r.session.modifiedTablesInTx = append(r.session.modifiedTablesInTx, decision.Tables...)
				} else {
					// Autocommit mode: invalidate tables immediately
					evicted := r.session.invalidator.InvalidateTables(decision.Tables)
					r.session.metrics.Invalidations.Add(1)
					r.session.metrics.EvictedEntries.Add(int64(len(evicted)))
					if r.session.tracker != nil {
						for _, t := range decision.Tables {
							r.session.tracker.RecordInvalidation(t, string(decision.Kind), len(evicted), elapsedMs)
						}
					}

					r.session.hub.Broadcast(telemetry.QueryEvent{
						SQL:        rawSQL,
						Status:     "INVALIDATE",
						DurationMs: elapsedMs,
						Tables:     decision.Tables,
					})
				}
			}

			// Handle COMMIT / ROLLBACK
			if r.session.txStatus == wire.TxStatusInTx && newTxStatus == wire.TxStatusIdle {
				if decision.Kind == parser.StmtCommit && len(r.session.modifiedTablesInTx) > 0 {
					evicted := r.session.invalidator.InvalidateTables(r.session.modifiedTablesInTx)
					r.session.metrics.Invalidations.Add(1)
					r.session.metrics.EvictedEntries.Add(int64(len(evicted)))
					if r.session.tracker != nil {
						for _, t := range r.session.modifiedTablesInTx {
							r.session.tracker.RecordInvalidation(t, "COMMIT", len(evicted), elapsedMs)
						}
					}

					r.session.hub.Broadcast(telemetry.QueryEvent{
						SQL:        rawSQL,
						Status:     "INVALIDATE",
						DurationMs: elapsedMs,
						Tables:     r.session.modifiedTablesInTx,
					})
				}
				r.session.modifiedTablesInTx = nil
			}

			r.session.txStatus = newTxStatus

			if decision.Kind == parser.StmtSelect && !decision.Cacheable {
				r.session.metrics.CacheBypasses.Add(1)
				r.session.hub.Broadcast(telemetry.QueryEvent{
					SQL:        rawSQL,
					Status:     "BYPASS",
					DurationMs: elapsedMs,
					Tables:     decision.Tables,
					Reason:     decision.Reason,
				})
			}

			return nil
		}
	}
}

func (r *Router) handleParse(msg wire.RawMessage) error {
	name, sql, oids := parseParseMessage(msg.Payload)
	r.session.mu.Lock()
	r.session.preparedStmts[name] = &PreparedStatement{
		Name:      name,
		SQL:       sql,
		ParamOIDs: oids,
	}
	r.session.mu.Unlock()

	// Forward to upstream
	if err := r.session.upstream.Writer.WriteRaw(msg); err != nil {
		return err
	}
	return r.session.upstream.Writer.Flush()
}

func (r *Router) handleBind(msg wire.RawMessage) error {
	portalName, stmtName, params := parseBindParameters(msg.Payload)
	r.session.mu.Lock()
	r.session.portals[portalName] = &Portal{
		Name:          portalName,
		StatementName: stmtName,
		Parameters:    params,
	}
	r.session.mu.Unlock()

	if err := r.session.upstream.Writer.WriteRaw(msg); err != nil {
		return err
	}
	return r.session.upstream.Writer.Flush()
}

func (r *Router) handleExtendedPassthrough(msg wire.RawMessage) error {
	if err := r.session.upstream.Writer.WriteRaw(msg); err != nil {
		return err
	}
	if err := r.session.upstream.Writer.Flush(); err != nil {
		return err
	}

	// Read upstream responses until ReadyForQuery or CommandComplete / BindComplete
	for {
		resp, err := r.session.upstream.Reader.ReadMessage()
		if err != nil {
			return err
		}

		if err := r.session.clientWriter.WriteRaw(resp); err != nil {
			return err
		}

		switch resp.Type {
		case wire.MsgReadyForQuery:
			if len(resp.Payload) > 0 {
				r.session.txStatus = resp.Payload[0]
			}
			return r.session.clientWriter.Flush()
		case wire.MsgParseComplete, wire.MsgBindComplete, wire.MsgCloseComplete, wire.MsgNoData:
			return r.session.clientWriter.Flush()
		case wire.MsgErrorResponse:
			_ = r.session.clientWriter.Flush()
			return nil
		}
	}
}
