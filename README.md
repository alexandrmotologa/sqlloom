<p align="center">
  <img src="docs/images/logo.png?raw=true" alt="sqlloom Logo" width="130" style="border-radius: 28px;" />
</p>

<h1 align="center">sqlloom</h1>

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.23-00ADD8?style=flat&logo=go" alt="Go 1.23" />
  <img src="https://img.shields.io/badge/React-19-61DAFB?style=flat&logo=react" alt="React 19" />
  <img src="https://img.shields.io/badge/Wire_Protocol-PostgreSQL_3.0-336791?style=flat&logo=postgresql" alt="PostgreSQL 3.0 Wire Protocol" />
  <img src="https://img.shields.io/badge/Memory-%3C_35MB_RAM-10b981?style=flat" alt="Memory Under 35MB RAM" />
  <img src="https://img.shields.io/badge/Read_Latency-%3C_0.5ms-10b981?style=flat" alt="Sub-0.5ms Read Latency" />
  <img src="https://img.shields.io/badge/License-MIT-blue.svg" alt="License" />
</p>

<p align="center">
  Transparent PostgreSQL wire-protocol query cache and read acceleration proxy with automated write invalidation under 35MB RAM.
</p>

---

<p align="center">
  <img src="docs/images/sqlloom_demo.gif" alt="sqlloom Live Query Cockpit and Acceleration Demo" width="100%" />
</p>

sqlloom sits between your application and your PostgreSQL database, speaking the native PostgreSQL 3.0 network protocol on port 5432 or 5433. It intercepts incoming SELECT queries, serves matching results from an in-memory striped LRU cache in under 0.5 milliseconds, and automatically invalidates cached queries whenever an INSERT, UPDATE, DELETE, or TRUNCATE touches dependent tables.

It requires zero application code changes, zero ORM modifications, and ships as a single self-contained binary with an embedded React 19 query intelligence cockpit.

---

## Live Cockpit & Responsive Interface

sqlloom embeds a real-time management console served directly by the binary on `:8080`.

### Desktop Cockpit
![sqlloom Desktop Dashboard](docs/images/sqlloom_dashboard.png)

### Table Dependency & Invalidation Map
![sqlloom Table Map](docs/images/sqlloom_table_map.png)

### Declarative Rules & Policies Manager
![sqlloom Rules Manager](docs/images/sqlloom_rules.png)

### Write Invalidation & Churn Activity Log
![sqlloom Invalidation Log](docs/images/sqlloom_invalidations.png)

### Integrated Smoke Benchmark & Load Simulator
![sqlloom Smoke Benchmark](docs/images/sqlloom_benchmark.png)

### Mobile & Tablet Adaptive Layouts
<p align="center">
  <img src="docs/images/sqlloom_mobile.png" alt="sqlloom Mobile Interface" width="32%" />
  &nbsp;&nbsp;
  <img src="docs/images/sqlloom_tablet.png" alt="sqlloom Tablet Interface" width="62%" />
</p>

---

## Key Capabilities

- **Zero application code changes**: applications connect directly to sqlloom using standard database drivers (psql, pgx, node-postgres, prisma, hibernate, gorm).
- **Sub-0.5ms read latency**: serves repetitive queries directly from memory, eliminating database query planning, lock contention, and disk I/O.
- **Automated table invalidation**: writes touching table T instantly purge all cached queries dependent on table T.
- **SingleFlight coalescing**: deduplicates concurrent cache misses in flight, suppressing thundering herd problems during cold starts.
- **Declarative cache rules engine**: configure table exclusions, per-table custom TTLs, and regex pattern routing at runtime without code changes.
- **Cache churn inspector**: detects high-frequency write tables and provides actionable recommendations to prevent cache thrashing.
- **Grace mode**: serves stale cache entries during transient upstream database spikes or brief connectivity hiccups.
- **Transaction safe**: queries inside explicit transactions (BEGIN ... COMMIT) bypass shared cache to preserve ACID read consistency. Invalidation occurs upon COMMIT.
- **Low memory footprint (< 35MB RAM)**: 16-shard thread-safe LRU cache with byte-level memory tracking and automatic eviction.
- **Embedded React 19 Cockpit**: live Server-Sent Events (SSE) query stream, hit ratio gauges, interactive rules editor, invalidation activity log, smoke benchmark load tester, and 1-click table purge console served directly on `:8080`.
- **CLI diagnostics**: command-line `sqlloom explain "<QUERY>"` for instant cacheability, fingerprint, and dependency inspection.
- **Prometheus telemetry**: metrics endpoint at `/metrics` exporting hit ratios, query execution durations, saved database time, and active connections.

---

## Architecture

```
[ Application (Node.js, Go, Python, Ruby, PHP, Java, Rust) ]
                       │
                       │  PostgreSQL Wire Protocol (:5433)
                       ▼
┌────────────────────────────────────────────────────────────────────────┐
│                        sqlloom Proxy Engine                            │
│                                                                        │
│  ┌──────────────────────────────────────────────────────────────────┐  │
│  │ Wire Protocol Decoder & Handshake Coordinator                    │  │
│  │ - SSLRequest pass-through & plain TCP negotiation                │  │
│  │ - Simple Query ('Q') and Extended Query ('P','B','E','S')        │  │
│  └──────────────────────────────────┬───────────────────────────────┘  │
│                                     │                                  │
│                                     ▼                                  │
│  ┌──────────────────────────────────────────────────────────────────┐  │
│  │ SQL Lexer & Table Dependency Extractor                           │  │
│  │ - Fingerprint: SHA-256(Database + NormalizedSQL + Params)        │  │
│  │ - Declarative Rules Filter: Exclusions, Patterns, Custom TTLs    │  │
│  │ - Cacheability Filter: volatile functions, tx status, directives │  │
│  └──────────────────────────────────┬───────────────────────────────┘  │
│                                     │                                  │
│         ┌───────────────────────────┴──────────────────────────┐       │
│         │ Is Cacheable SELECT Query?                           │ Is Write / Uncacheable?
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
│  │ - Interactive Rules Editor (table exclusions, TTLs, regexes)     │  │
│  │ - Invalidation Activity Log & Cache Churn Inspector              │  │
│  │ - 1-Click Smoke Benchmark & Synthetic Load Simulator             │  │
│  │ - Prometheus metrics (/metrics) and health probe (/healthz)      │  │
│  └──────────────────────────────────────────────────────────────────┘  │
└────────────────────────────────────────────────────────────────────────┘
```

---

## Quickstart

### 1. Run via Docker Compose

```bash
docker compose up -d
```

This starts:
- Upstream PostgreSQL 16 on internal port 5432.
- `sqlloom` proxy listening on port 5432 (proxy) and port 8080 (cockpit).

### 2. Run Standalone Binary

```bash
# Build binary
go build -ldflags="-s -w" -o sqlloom ./cmd/sqlloom

# Start proxy pointing to your PostgreSQL instance
./sqlloom -listen :5433 -upstream 127.0.0.1:5432 -dashboard :8080
```

### 3. Connect Applications

Point any database connection string to `sqlloom` on port 5433:

```bash
# Direct psql connection
psql "postgres://user:password@localhost:5433/mydb"

# Node.js pg
DATABASE_URL="postgresql://user:password@localhost:5433/mydb"

# Python psycopg2 / SQLAlchemy
DATABASE_URL="postgresql+psycopg2://user:password@localhost:5433/mydb"
```

Open `http://localhost:8080` in any web browser to view queries, hit ratios, and table dependencies streaming live.

---

## CLI Diagnostics

Inspect how sqlloom analyzes, normalizes, and routes any query without starting a database connection:

```bash
sqlloom explain "SELECT id, name, email FROM users WHERE id = 42"
```

Output:
```text
=== sqlloom Query Diagnostics & Fingerprint ===
Raw SQL:              SELECT id, name, email FROM users WHERE id = 42
Statement Kind:       SELECT
Cacheable:            YES (Eligible for < 0.5ms acceleration)
Normalized Template:  select id, name, email from users where id = ?
Deterministic Key:    0c3408cc7b8b8ac2d3b9cc778219914ad33287fb785075b13f31442fe6cbfff3
Target LRU Shard:     Shard #7 (of 16 striped shards)
Dependent Tables:     [users public.users]
=================================================
```

---

## Configuration

Flags and environment variables:

| Flag | Env Variable | Default | Description |
| :--- | :--- | :--- | :--- |
| `-listen` | `SQLLOOM_LISTEN` | `:5433` | TCP address for client connections |
| `-upstream` | `SQLLOOM_UPSTREAM` | `127.0.0.1:5432` | Upstream PostgreSQL server address |
| `-dashboard` | `SQLLOOM_DASHBOARD` | `:8080` | HTTP Cockpit and Prometheus metrics address |
| `-cache-mem-mb` | `SQLLOOM_CACHE_MEM_MB` | `32` | In-memory cache size limit in megabytes |
| `-cache-entries`| `SQLLOOM_CACHE_ENTRIES`| `10000` | Maximum cached query entries limit |
| `-ttl` | `SQLLOOM_TTL_SEC` | `3600` | Default cache TTL in seconds (0 for indefinite) |
| `-grace-mode` | `SQLLOOM_GRACE_MODE` | `true` | Serve stale cache on upstream database errors |
| `-disable-cache`| `SQLLOOM_DISABLE_CACHE`| `false` | Pass-through proxy mode without caching |
| `-verbose` | `SQLLOOM_VERBOSE` | `false` | Enable verbose wire protocol debug logging |

---

## Cache Directives

Developers can control cache behavior directly in SQL comments:

```sql
-- Force cache bypass (always fetch fresh from PostgreSQL)
/* sqlloom:nocache */ SELECT id, balance FROM accounts WHERE id = 100;

-- Custom TTL override (cache for 30 seconds only)
/* sqlloom:ttl=30 */ SELECT count(*) FROM active_visitors;
```

---

## Testing

Run the full test suite:

```bash
go test -v ./...
```

The test suite covers:
- PostgreSQL 3.0 wire protocol packet serialization and deserialization
- SQL lexer query normalization and parameter stripping
- Table dependency extraction across SELECT, JOIN, INSERT, UPDATE, DELETE, and TRUNCATE
- 16-shard thread-safe LRU cache evictions, expirations, and byte accounting
- SingleFlight concurrent query deduplication
- Instant table invalidation and ACID transaction commit invalidation
- End-to-end proxy integration tests with mock PostgreSQL upstream

---

## License

MIT License. Designed and engineered by [Alexandr Motologa](https://github.com/alexandrmotologa).
