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
│  │ - Cacheability Engine: Detects now(), random(), tx status, locks │  │
│  └──────────────────────────────────┬───────────────────────────────┘  │
│                                     │                                  │
│         ┌───────────────────────────┴──────────────────────────┐       │
│         │ Is Cacheable SELECT Query?                           │ Is Write / Non-Cacheable?
│         ▼                                                      ▼       │
│  ┌───────────────────────────────┐           ┌──────────────────────┐  │
│  │ In-Memory Sharded Query Cache │           │ Forward to Postgres  │  │
│  │ (RowDescription + DataRows)   │           │ & Invalidate Tables  │  │
│  └──────────────┬────────────────┘           └──────────┬───────────┘  │
│   Cache HIT     │          │ Cache MISS                 │              │
│   (< 0.5ms)     │          ▼                            │              │
│         ┌───────┘   ┌──────────────────────────────┐    │              │
│         │           │ Forward Query to Upstream PG │    │              │
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
│  │ - Table dependency viewer and 1-click table-level cache purge    │  │
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
| **Caching Engine** | Striped LRU Cache | 16-shard thread-safe in-memory cache with memory byte tracking and automated LRU eviction |
| **Telemetry and API** | HTTP Server + SSE Stream | Real-time SSE query log, Prometheus `/metrics`, JSON health and purge endpoints |
| **Embedded Web UI** | React 19 + TypeScript + Vite | Observability console compiled to static assets and embedded into Go binary with `//go:embed` |
| **UI Styling** | Tailwind CSS + Lucide Icons | Dense dark theme dashboard with real-time charts and table inspector |

---

## 4. Directory Structure

```
B:\workgit\sqlloom\
├── cmd\
│   └── sqlloom\
│       └── main.go                 # Proxy entrypoint, CLI flags, configuration bootstrapper
├── internal\
│   ├── config\
│   │   └── config.go               # Configuration (upstream host/port, listen port, cache size, http port)
│   ├── wire\
│   │   ├── protocol.go             # PostgreSQL 3.0 protocol constants and message types
│   │   ├── reader.go               # Streaming packet deserializer (frontend/backend)
│   │   ├── writer.go               # Streaming packet serializer and buffer writer
│   │   └── auth.go                 # Authentication pass-through and handshake coordinator
│   ├── proxy\
│   │   ├── server.go               # Ingress TCP listener and graceful shutdown
│   │   ├── session.go              # Client connection session state, transaction tracker
│   │   ├── upstream.go             # Upstream PostgreSQL connection manager
│   │   └── router.go               # Query routing, execution coordination, cache interceptor
│   ├── parser\
│   │   ├── fingerprint.go          # Query normalization and deterministic cache key hashing
│   │   ├── tables.go               # Table dependency extractor (FROM, JOIN, INTO, UPDATE)
│   │   └── rules.go                # Cacheability filter (tx status, functions, locks, directives)
│   ├── cache\
│   │   ├── sharded_lru.go          # 16-shard memory-bounded LRU cache
│   │   ├── entry.go                # Serialized RowDescription + DataRow cache record
│   │   └── invalidator.go          # Table-to-query reverse index and invalidator
│   └── telemetry\
│       ├── metrics.go              # Prometheus collectors and counters
│       ├── event_hub.go            # SSE event pub-sub hub for real-time UI stream
│       └── http_server.go          # Cockpit HTTP server, API routes, and embedded assets
├── web\
│   ├── src\
│   │   ├── components\
│   │   │   ├── MetricCards.tsx     # Hit ratio, saved database query time, active cached entries
│   │   │   ├── QueryStream.tsx     # Chronological query stream showing cached vs upstream execution
│   │   │   ├── TableMap.tsx        # Visual table dependency map with query counts
│   │   │   ├── QueryDetail.tsx     # Query fingerprint, parameters, raw SQL, execution delta
│   │   │   └── EvictionControl.tsx # Manual table and global cache purge triggers
│   │   ├── types.ts                # TypeScript interfaces for query telemetry and stats
│   │   ├── App.tsx                 # Root dashboard layout and tab navigation
│   │   ├── main.tsx                # React 19 entrypoint
│   │   └── index.css               # Tailwind CSS declarations
│   ├── package.json
│   ├── tsconfig.json
│   └── vite.config.ts
├── test\
│   ├── mock_upstream.go            # In-memory mock PostgreSQL server for fast unit/integration testing
│   └── proxy_test.go               # End-to-end integration tests (Simple & Extended protocol)
├── docs\
│   ├── ARCHITECTURE.md             # Detailed wire protocol and invalidation mechanics
│   └── BENCHMARK.md                # Latency and throughput benchmarks
├── Makefile                        # Compilation, tests, and web asset build pipeline
├── Dockerfile                      # Scratch container image (< 25MB)
├── README.md                       # Documentation, quickstart guide, and configuration reference
└── MASTER_PLAN.md                  # This master specification
```

---

## 5. Invalidation Mechanics and Query Fingerprinting

### 1. Deterministic Query Fingerprint
For any incoming query statement:
1. Normalize SQL statement: strip whitespace, comments, convert literal values to positional placeholders (`?`), and lowercase keywords.
2. Extract bind parameters: for Simple Query, literals are normalized; for Extended Query, binary or text parameter buffers are appended.
3. Compute SHA-256 hash over `(database_name, normalized_sql, serialized_parameters)`.

### 2. Cacheability Rules
A query is eligible for caching only when all conditions hold:
- Statement kind is `SELECT`.
- Not containing write or locking clauses (`FOR UPDATE`, `FOR SHARE`, `INTO`).
- Not containing non-deterministic volatile functions (`now()`, `CURRENT_TIMESTAMP`, `random()`, `uuid_generate_v4()`, `gen_random_uuid()`, `clock_timestamp()`).
- Not executing inside an active explicit transaction (`BEGIN` / `COMMIT`) unless explicit comment directive permits.
- Does not target internal system catalogs (`pg_*`, `information_schema`).
- Does not contain comment directive `/* sqlloom:nocache */`.

### 3. Automated Table Invalidation Matrix

| Incoming Statement | Action Taken by sqlloom |
| :--- | :--- |
| `SELECT ... FROM products` | Check cache. If HIT, stream cached RowDescription and DataRows (< 0.5ms). If MISS, forward to upstream, cache rows, and stream back. |
| `UPDATE products SET price = 99 ...` | Forward to upstream PG. On `CommandComplete ('C')`, immediately purge all cache keys dependent on table `products`. |
| `INSERT INTO orders ...` | Forward to upstream PG. Purge all cache keys dependent on table `orders`. |
| `DELETE FROM customers ...` | Forward to upstream PG. Purge all cache keys dependent on table `customers`. |
| `TRUNCATE TABLE logs` | Forward to upstream PG. Purge all cache keys dependent on table `logs`. |
| `ALTER TABLE ... / DROP TABLE ...` | Forward to upstream PG. Purge all cache keys dependent on touched table or flush all cache if DDL drops schema. |
| `BEGIN ... COMMIT` | Queries inside explicit transactions bypass shared cache to preserve ACID read consistency. On successful `COMMIT`, invalidate all tables modified in the transaction. |

---

## 6. Implementation Roadmap and Task Breakdown

### Phase 0: Project Setup and Repository Hygiene
- [x] Create public GitHub repository `alexandrmotologa/sqlloom`.
- [x] Configure repository metadata and 12 targeted topics across 4 dimensions.
- [x] Register project in MTLG portfolio hub (`mtlg.site` and `mtlglabs.space`) under Active Development.
- [x] Scaffold repository layout and initialize Go module `github.com/alexandrmotologa/sqlloom`.

### Phase 1: Wire Protocol Engine and Transparent TCP Proxy
- [ ] Implement `internal/wire/protocol.go`: message byte identifiers ('Q', 'P', 'B', 'D', 'E', 'S', 'C', 'Z', 'T', 'D', 'R', 'E', 'N', 'K'), constants, and payload structures.
- [ ] Implement `internal/wire/reader.go`: streaming packet deserializer reading length-prefixed protocol frames.
- [ ] Implement `internal/wire/writer.go`: memory-efficient streaming frame writer with buffer pooling.
- [ ] Implement `internal/wire/auth.go`: client SSL negotiation handler and authentication pass-through between client and upstream.
- [ ] Implement `internal/proxy/session.go` and `internal/proxy/upstream.go`: connection management, session state machine, and transaction status tracker.

### Phase 2: SQL Lexer, Parser and Table Dependency Extractor
- [ ] Implement `internal/parser/fingerprint.go`: SQL normalization, parameter hashing, and deterministic cache key generation.
- [ ] Implement `internal/parser/tables.go`: robust table extraction from `FROM`, `JOIN`, `UPDATE`, `INSERT INTO`, `DELETE FROM`, and `TRUNCATE`.
- [ ] Implement `internal/parser/rules.go`: cacheability engine checking non-deterministic functions, locking keywords, transaction state, and comment directives (`/* sqlloom:nocache */`, `/* sqlloom:ttl=X */`).

### Phase 3: Sharded LRU Cache and Automatic Invalidator
- [ ] Implement `internal/cache/entry.go`: serialized `RowDescription` and `DataRow` packet slices with memory size accounting.
- [ ] Implement `internal/cache/sharded_lru.go`: 16-shard thread-safe cache with memory threshold enforcement (< 35MB default ceiling).
- [ ] Implement `internal/cache/invalidator.go`: bidirectional table-to-cache-key index for instant table invalidation upon write statements.
- [ ] Implement write-intercept logic in proxy router: on `CommandComplete` for write statements, trigger immediate table invalidation.

### Phase 4: Extended Query Protocol and Multi-Client Support
- [ ] Support Extended Query Protocol ('P' Parse, 'B' Bind, 'D' Describe, 'E' Execute, 'S' Sync) alongside Simple Query ('Q').
- [ ] Implement prepared statement tracking and bind parameter extraction for ORMs (Prisma, Drizzle, Hibernate, GORM, pgx).
- [ ] Transaction isolation handling: ensure writes within uncommitted transactions do not invalidate until commit, and queries in transactions read consistently.

### Phase 5: Telemetry, Observability API and Real-Time Event Hub
- [ ] Implement `internal/telemetry/metrics.go`: Prometheus metrics (`sqlloom_query_total`, `sqlloom_cache_hits_total`, `sqlloom_cache_misses_total`, `sqlloom_query_duration_seconds`, `sqlloom_memory_bytes`, `sqlloom_invalidation_events_total`).
- [ ] Implement `internal/telemetry/event_hub.go`: lock-free pub-sub event broadcaster for real-time query events.
- [ ] Implement `internal/telemetry/http_server.go`: REST API endpoints (`/api/status`, `/api/metrics`, `/api/tables`, `/api/cache/purge`) and SSE endpoint (`/api/events`).

### Phase 6: Embedded React 19 Query Cockpit
- [ ] Scaffold Vite + React 19 + TypeScript + Tailwind CSS application in `web/`.
- [ ] Build high-density dark mode dashboard:
  - Metric cards: hit ratio %, query rate (QPS), saved database latency, cache memory usage.
  - Live query stream with search, filter tabs (ALL, HIT, MISS, BYPASS, INVALIDATE), and query latency tags.
  - Table dependency map with cached query counts and 1-click purge button per table.
  - Query details drawer showing raw SQL, normalized template, parameters, and cache status.
- [ ] Integrate React build artifacts into Go binary using `//go:embed` to serve directly from the single binary.

### Phase 7: Verification, Benchmarking, Documentation and Packaging
- [ ] Implement automated integration test suite with mock PostgreSQL upstream and live protocol testing.
- [ ] Run latency and memory profiling to verify < 0.5ms cache hit latency and < 35MB RAM footprint.
- [ ] Write comprehensive `README.md` and `docs/ARCHITECTURE.md` adhering strictly to humanizer principles.
- [ ] Create multi-stage `Dockerfile` producing a scratch container binary (< 25MB) and `Makefile`.
