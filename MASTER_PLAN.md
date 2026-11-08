# sqlloom: Architecture and Engineering Master Plan

> Transparent PostgreSQL Wire-Protocol Query Cache and Read Acceleration Proxy (< 35MB RAM)
> Alternative to ReadySet, PolyScale, and Prisma Accelerate in a Single Self-Contained Binary (Go + React 19)

---

## 1. Executive Summary and Problem Space

### The Core Problem
In data-driven software, repetitive `SELECT` queries (tenant settings, product catalogs, user profiles, permission lookups, feature flags) make up over 80% of database traffic. These queries consume database CPU cycles, increase connection pressure, and drive cloud database infrastructure costs.

Engineering teams typically face two options:
1. Managed cloud query accelerators (ReadySet, PolyScale, Prisma Accelerate): cost $150 to $600+ monthly, introduce third-party network hops, and require routing production database credentials through external providers.
2. Manual application caching with Redis or Memcached: requires writing repetitive caching logic across services (`cache.get(key) || db.query(...)`). This approach is prone to stale data bugs, race conditions, and synchronization failures whenever writes occur.

### The Solution: sqlloom
`sqlloom` is a lightweight, single-binary PostgreSQL wire-protocol proxy written in Go with an embedded React 19 observability cockpit. It listens on port 5432 (or configurable port), speaking the native PostgreSQL frontend and backend protocol (version 3.0). Applications connect to `sqlloom` as if it were a standard PostgreSQL database, requiring zero application code changes and zero ORM alterations.

`sqlloom` intercepts client queries, checks an in-memory striped LRU cache, and serves matching `SELECT` queries in under 0.5 milliseconds. When an `INSERT`, `UPDATE`, `DELETE`, or `TRUNCATE` query executes, `sqlloom` forwards the write to upstream PostgreSQL, waits for successful completion, and instantly purges all cached queries that depend on the touched tables.

---

## 2. System Architecture and Request Lifecycle

```
[ Application (Node.js, Go, Python, Ruby, PHP, Java, Rust) ]
                       │
                       │  PostgreSQL Wire Protocol (:5432 / :5433)
                       ▼
┌────────────────────────────────────────────────────────────────────────┐
│                        sqlloom Proxy Engine                            │
│                                                                        │
│  ┌──────────────────────────────────────────────────────────────────┐  │
│  │ Wire Protocol Decoder and Connection Handler                     │  │
│  │ - SSL/TLS Handshake negotiation (SSLRequest pass-through)        │  │
│  │ - Authentication pass-through (Cleartext, MD5, SCRAM-SHA-256)    │  │
│  │ - Simple Query ('Q') and Extended Query Protocol ('P','B','E')   │  │
│  └──────────────────────────────────┬───────────────────────────────┘  │
│                                     │                                  │
│                                     ▼                                  │
│  ┌──────────────────────────────────────────────────────────────────┐  │
│  │ SQL Lexer, Parser and Table Dependency Extractor                 │  │
│  │ - Query Fingerprint: SHA-256(NormalizedAST + BindParams)         │  │
│  │ - Dependency Extraction: Tables from FROM, JOIN, INTO, UPDATE    │  │
│  │ - Declarative Policies: Table exclusions, regex routing, TTLs    │  │
│  │ - Cacheability Engine: Detects now(), random(), tx status, locks │  │
│  └──────────────────────────────────┬───────────────────────────────┘  │
│                                     │                                  │
│         ┌───────────────────────────┴──────────────────────────┐       │
│         │ Is Cacheable SELECT Query?                           │ Is Write / Non-Cacheable?
│         ▼                                                      ▼       │
│  ┌───────────────────────────────┐           ┌──────────────────────┐  │
│  │ In-Memory Sharded Query Cache │           │ Forward to Postgres  │  │
│  │ (RowDescription + DataRows)   │           │ & Invalidate Tables  │  │
│  │ SingleFlight In-Flight Guard  │           │ Track Churn Activity │  │
│  └──────────────┬────────────────┘           └──────────┬───────────┘  │
│   Cache HIT     │          │ Cache MISS                 │              │
│   (< 0.5ms)     │          ▼                            │              │
│         ┌───────┘   ┌──────────────────────────────┐    │              │
│         │           │ Forward Query to Upstream PG │    │              │
│         │           │ (SingleFlight deduplication) │    │              │
│         │           └──────────────┬───────────────┘    │              │
│         │                          ▼                    │              │
│         │           ┌──────────────────────────────┐    │              │
│         │           │ Intercept Rows & Store Cache │    │              │
│         │           └──────────────┬───────────────┘    │              │
│         ▼                          ▼                    ▼              │
│  [ Stream Result Back to Client: RowDescription + DataRows + Complete ]│
│                                                                        │
│  ┌──────────────────────────────────────────────────────────────────┐  │
│  │ Embedded React 19 Query Cockpit (:8080)                          │  │
│  │ - Real-time SSE query stream, hit/miss ratios, saved latency     │  │
│  │ - Declarative Rules Manager: exclusions, TTLs, regex policies    │  │
│  │ - Invalidation Activity Log & Cache Churn Inspector              │  │
│  │ - 1-Click Smoke Benchmark & Synthetic Load Simulator             │  │
│  │ - Prometheus metrics (/metrics) and health probe (/healthz)      │  │
│  └──────────────────────────────────────────────────────────────────┘  │
└────────────────────────────────────────────────────────────────────────┘
```

---

## 3. Technology Stack

| Layer | Component | Specification |
| :--- | :--- | :--- |
| **Language and Runtime** | Go 1.23+ | High-throughput concurrent network I/O, low latency, < 35MB RAM footprint |
| **Protocol Parser** | Native PG Wire Decoder | Implements PostgreSQL Frontend/Backend protocol 3.0: `StartupMessage`, `SSLRequest`, `Query`, `Parse`, `Bind`, `Describe`, `Execute`, `Sync`, `RowDescription`, `DataRow`, `CommandComplete`, `ReadyForQuery` |
| **SQL Parser and AST** | SQL Lexer and AST Extractor | Fast statement analyzer extracting statement kind (SELECT, INSERT, UPDATE, DELETE, TRUNCATE, BEGIN, COMMIT), referenced table names, and cacheability barriers |
| **Concurrency Guard** | SingleFlight Group | In-flight request deduplication suppressing thundering herd problems |
| **Caching Engine** | Striped LRU Cache | 16-shard thread-safe in-memory cache with memory byte tracking and automated LRU eviction |
| **Declarative Rules** | Rules Engine | Dynamic table exclusion lists, custom table TTL overrides, and SQL regex rules |
| **Churn Tracker** | Invalidation Monitor | Write frequency analysis and churn warnings for high-invalidation tables |
| **Telemetry and API** | HTTP Server + SSE Stream | Real-time SSE query log, Prometheus `/metrics`, JSON health and purge endpoints |
| **Embedded Web UI** | React 19 + TypeScript + Vite | Observability console compiled to static assets and embedded into Go binary with `//go:embed` |
| **UI Styling** | Tailwind CSS + Lucide Icons | Dense dark theme dashboard with real-time charts and table inspector |

---

## 4. Implementation Roadmap and Status

### Phase 0: Project Setup and Repository Hygiene
- [x] Create public GitHub repository `alexandrmotologa/sqlloom`.
- [x] Configure repository metadata and 12 targeted topics across 4 dimensions.
- [x] Register project in MTLG portfolio hub (`mtlg.site` and `mtlglabs.space`) under Active Development.
- [x] Scaffold repository layout and initialize Go module `github.com/alexandrmotologa/sqlloom`.

### Phase 1: Wire Protocol Engine and Transparent TCP Proxy
- [x] Implement `internal/wire/protocol.go`: message byte identifiers, constants, and payload structures.
- [x] Implement `internal/wire/reader.go`: streaming packet deserializer reading length-prefixed protocol frames.
- [x] Implement `internal/wire/writer.go`: memory-efficient streaming frame writer with buffer pooling.
- [x] Implement `internal/wire/auth.go`: client SSL negotiation handler and authentication pass-through between client and upstream.
- [x] Implement `internal/proxy/session.go` and `internal/proxy/upstream.go`: connection management, session state machine, and transaction status tracker.

### Phase 2: SQL Lexer, Parser and Table Dependency Extractor
- [x] Implement `internal/parser/fingerprint.go`: SQL normalization, parameter hashing, and deterministic cache key generation.
- [x] Implement `internal/parser/tables.go`: robust table extraction from `FROM`, `JOIN`, `UPDATE`, `INSERT INTO`, `DELETE FROM`, and `TRUNCATE`.
- [x] Implement `internal/parser/rules.go`: cacheability engine checking non-deterministic functions, locking keywords, transaction state, and comment directives.

### Phase 3: Sharded LRU Cache and Automatic Invalidator
- [x] Implement `internal/cache/entry.go`: serialized `RowDescription` and `DataRow` packet slices with memory size accounting.
- [x] Implement `internal/cache/sharded_lru.go`: 16-shard thread-safe cache with memory threshold enforcement (< 35MB default ceiling).
- [x] Implement `internal/cache/invalidator.go`: bidirectional table-to-cache-key index for instant table invalidation upon write statements.
- [x] Implement write-intercept logic in proxy router: on `CommandComplete` for write statements, trigger immediate table invalidation.

### Phase 4: Extended Query Protocol and Multi-Client Support
- [x] Support Extended Query Protocol ('P' Parse, 'B' Bind, 'D' Describe, 'E' Execute, 'S' Sync) alongside Simple Query ('Q').
- [x] Implement prepared statement tracking and bind parameter extraction for ORMs.
- [x] Transaction isolation handling: ensure writes within uncommitted transactions do not invalidate until commit, and queries in transactions read consistently.

### Phase 5: Telemetry, Observability API and Real-Time Event Hub
- [x] Implement `internal/telemetry/metrics.go`: Prometheus metrics.
- [x] Implement `internal/telemetry/event_hub.go`: lock-free pub-sub event broadcaster for real-time query events.
- [x] Implement `internal/telemetry/http_server.go`: REST API endpoints and SSE endpoint (`/api/events`).

### Phase 6: Embedded React 19 Query Cockpit
- [x] Scaffold Vite + React 19 + TypeScript + Tailwind CSS application in `web/`.
- [x] Build Metric cards, Live query stream, and Table dependency map with 1-click purge.
- [x] Embed static build assets into Go binary with `//go:embed`.

### Phase 7: Advanced Engineering Enhancements (Production Features)
- [x] Implement `internal/proxy/singleflight.go`: in-flight query deduplication preventing thundering herd cache stampedes.
- [x] Implement `internal/parser/declarative_rules.go`: dynamic table exclusion lists, custom table TTL overrides, and SQL regex routing.
- [x] Implement `internal/telemetry/invalidation_tracker.go`: real-time invalidation activity log and high-churn table detection with recommendations.
- [x] Implement `internal/telemetry/benchmark.go`: built-in concurrent smoke benchmark load simulator.
- [x] Implement CLI `sqlloom explain "<QUERY>"`: command-line query analyzer and fingerprint diagnostics.
- [x] Implement Grace Mode: serve stale cache during upstream hiccups.
- [x] Build React 19 UI tabs for Rules & Policies, Invalidation Activity, and Smoke Benchmark modal.

### Phase 8: Verification, Benchmarking, Documentation and Packaging
- [x] Implement automated integration test suite with mock PostgreSQL upstream and live protocol testing.
- [x] Run latency and memory profiling to verify < 0.5ms cache hit latency and < 35MB RAM footprint.
- [x] Write comprehensive `README.md`, `docs/ARCHITECTURE.md`, `docs/BENCHMARK.md`.
- [x] Create multi-stage scratch `Dockerfile` and `docker-compose.yml`.
