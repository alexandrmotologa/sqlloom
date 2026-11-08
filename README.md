# sqlloom

> Transparent PostgreSQL wire-protocol query cache and read acceleration proxy under 35MB RAM.

sqlloom sits between your application and your PostgreSQL database, speaking the native PostgreSQL 3.0 network protocol. It intercepts incoming `SELECT` queries, serves matching results from an in-memory striped LRU cache in under 0.5 milliseconds, and automatically invalidates cached queries whenever an `INSERT`, `UPDATE`, `DELETE`, or `TRUNCATE` touches dependent tables.

It requires zero application code changes, zero ORM modifications, and ships as a single self-contained binary with an embedded React 19 query intelligence cockpit.

---

## Key Capabilities

- **Zero application code changes**: applications connect directly to sqlloom using standard database drivers (`psql`, `pgx`, `node-postgres`, `prisma`, `hibernate`, `gorm`).
- **Sub-0.5ms read latency**: serves repetitive queries directly from memory, eliminating database query planning, lock contention, and disk I/O.
- **Automated table invalidation**: writes touching table `T` instantly purge all cached queries dependent on table `T`.
- **Transaction safe**: queries inside explicit transactions (`BEGIN ... COMMIT`) bypass shared cache to preserve ACID read consistency. Invalidation occurs upon `COMMIT`.
- **Low memory footprint (< 35MB RAM)**: 16-shard thread-safe LRU cache with byte-level memory tracking and automatic eviction.
- **Embedded React 19 Cockpit**: live Server-Sent Events (SSE) query stream, hit ratio gauges, table dependency trees, and 1-click manual eviction console served directly from the single binary on `:8080`.
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
│  │ - Table Dependency Extraction: FROM, JOIN, INTO, UPDATE          │  │
│  │ - Cacheability Filter: volatile functions, tx status, directives │  │
│  └──────────────────────────────────┬───────────────────────────────┘  │
│                                     │                                  │
│         ┌───────────────────────────┴──────────────────────────┐       │
│         │ Is Cacheable SELECT Query?                           │ Is Write / Uncacheable?
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
make build

# Start proxy pointing to local PostgreSQL
./sqlloom -listen :5433 -upstream 127.0.0.1:5432 -dashboard :8080
```

### 3. Connect Any Client

Point your application database connection to the proxy port (`5433`):

```bash
# Using standard psql client
psql -h 127.0.0.1 -p 5433 -U postgres -d mydatabase

# Node.js connection string
DATABASE_URL="postgres://postgres:password@127.0.0.1:5433/mydatabase"

# Go database/sql connection
db, err := sql.Open("postgres", "host=127.0.0.1 port=5433 user=postgres dbname=mydatabase sslmode=disable")
```

Open the Query Cockpit in your browser at `http://localhost:8080`.

---

## Invalidation Matrix

| SQL Statement | Proxy Action |
| :--- | :--- |
| `SELECT ... FROM users WHERE id = 1` | Check cache. If HIT, return in under 0.5ms. If MISS, forward to PG, store result frames, and return. |
| `UPDATE users SET name = 'Bob' ...` | Forward to upstream PG. On `CommandComplete`, immediately purge all cached queries dependent on table `users`. |
| `INSERT INTO orders ...` | Forward to upstream PG. Purge all cached queries dependent on table `orders`. |
| `DELETE FROM sessions ...` | Forward to upstream PG. Purge all cached queries dependent on table `sessions`. |
| `TRUNCATE TABLE logs` | Forward to upstream PG. Purge all cached queries dependent on table `logs`. |
| `BEGIN ... COMMIT` | Queries inside transactions bypass cache for ACID consistency. Modified tables are invalidated on `COMMIT`. |
| `SELECT ... /* sqlloom:nocache */` | Query directive: bypasses cache unconditionally. |
| `SELECT ... /* sqlloom:ttl=60s */` | Query directive: overrides default TTL with 60 seconds. |

---

## Configuration Reference

| Flag | Environment Variable | Default | Description |
| :--- | :--- | :--- | :--- |
| `-listen` | `SQLLOOM_LISTEN` | `:5433` | TCP address for incoming PostgreSQL client connections |
| `-upstream` | `SQLLOOM_UPSTREAM` | `127.0.0.1:5432` | Upstream PostgreSQL server address |
| `-dashboard` | `SQLLOOM_DASHBOARD` | `:8080` | HTTP address for Query Cockpit UI and metrics |
| `-cache-mem-mb` | `SQLLOOM_CACHE_MAX_MEMORY_MB` | `32` | Maximum cache memory limit in megabytes |
| `-cache-entries` | `SQLLOOM_CACHE_MAX_ENTRIES` | `10000` | Maximum number of cached query entries |
| `-ttl` | `SQLLOOM_DEFAULT_TTL_SEC` | `3600` | Default cache entry TTL in seconds (0 for indefinite until write invalidation) |
| `-disable-cache` | `SQLLOOM_DISABLE_CACHE` | `false` | Run in pure pass-through proxy mode |
| `-verbose` | `SQLLOOM_VERBOSE` | `false` | Enable verbose protocol debug logs |

---

## Benchmarks Summary

Full benchmark methodology and raw figures are documented in [docs/BENCHMARK.md](file:///docs/BENCHMARK.md).

- **Cache Hit Latency**: 0.24ms p50 (vs 14.80ms upstream PostgreSQL).
- **Throughput**: 41,200 QPS on cached workloads (10x boost over unaccelerated baseline).
- **Memory Footprint**: 28.5 MB resident memory under 10,000 active cached queries.
- **Write Invalidation Overhead**: under 0.08 milliseconds.

---

## License

MIT License. Copyright (c) Alexandr Motologa.
