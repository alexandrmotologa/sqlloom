# sqlloom Architecture and Protocol Specification

sqlloom is a single binary PostgreSQL wire-protocol proxy with an embedded query cache and automated table invalidation engine. This document details its network framing, cache state machine, and concurrency model.

## 1. Network Protocol Flow

PostgreSQL frontend and backend communication uses length-prefixed packet frames over TCP. sqlloom intercepts client connections on its listen port and proxies requests to upstream PostgreSQL.

```
Client Connection
       │
       ▼
[SSLRequest Negotiation]
       │ Proxy responds 'N' (plain TCP proxying)
       ▼
[StartupMessage Relay]
       │ Passes database name, user, options to upstream
       ▼
[Authentication Exchange]
       │ Relays Cleartext, MD5, or SCRAM-SHA-256 packets
       ▼
[ReadyForQuery 'Z'] ── Connection Established
```

### Simple Query Protocol ('Q')
1. Client sends byte `'Q'` followed by int32 length and null-terminated SQL text.
2. sqlloom parses the SQL text and evaluates cacheability:
   - If cacheable and found in cache: streams cached `RowDescription` ('T'), `DataRow` ('D') frames, `CommandComplete` ('C'), and `ReadyForQuery` ('Z') to the client in under 0.5 milliseconds.
   - If cacheable and missed: forwards `'Q'` to upstream, streams response frames back to client, and stores the serialized packet buffers in the striped LRU cache.
   - If write query (`INSERT`, `UPDATE`, `DELETE`, `TRUNCATE`, `ALTER`, `DROP`): forwards to upstream, waits for `CommandComplete`, and purges all cached query keys tagged with the touched tables.

### Extended Query Protocol ('P', 'B', 'D', 'E', 'S')
- `Parse ('P')`: registers prepared statement and parameter types in the session state machine.
- `Bind ('B')`: binds parameters to a named portal.
- `Describe ('D')`: retrieves row description for a portal or statement.
- `Execute ('E')`: executes portal.
- `Sync ('S')`: completes extended batch and yields `ReadyForQuery ('Z')`.

## 2. Query Fingerprinting and Deterministic Hashing

Incoming SQL statements undergo normalization:
1. Comments (`-- ...` and `/* ... */`) are removed.
2. Directives (`/* sqlloom:nocache */` and `/* sqlloom:ttl=X */`) are parsed.
3. Literals (quoted strings and numbers) are normalized to positional placeholders (`?`).
4. Keywords are folded to lower case and whitespace is collapsed.

The cache key is computed as:
```
CacheKey = SHA256(DatabaseName + "\x00" + NormalizedSQL + "\x00" + SerializedParams)
```

## 3. Table Dependency and Invalidation Mechanics

sqlloom maintains a bidirectional lookup table:
- `tableToKeys`: maps table name to a set of active query cache keys.
- `keyToTables`: maps query cache key to the list of tables it references.

When writes occur:
- In autocommit mode: on `CommandComplete` for an `INSERT`, `UPDATE`, or `DELETE`, sqlloom immediately purges all keys indexed under the touched tables.
- In active transaction mode (`BEGIN ... COMMIT`): modified table names are held in a transaction buffer. On successful `COMMIT`, all buffered tables are purged together. If `ROLLBACK` occurs, the buffer is dropped without invalidating the cache.

## 4. Striped LRU Memory Management (< 35MB RAM)

The cache is partitioned into 16 independent shards using FNV-1a hashing on the cache key. Each shard holds:
- A `sync.RWMutex` protecting shard operations.
- A doubly linked list maintaining recency order.
- A hash table for O(1) key-to-node lookups.

Memory is tracked at the byte level, accounting for:
- 128 bytes base struct overhead.
- Key and SQL string lengths.
- Table dependency string arrays.
- Byte length of `RowDescription` and raw `DataRow` packet buffers.

When total cache memory exceeds the configured threshold (default 32MB) or the entry limit is reached, least recently used entries are evicted round-robin across shards.

## 5. Telemetry and Real-Time Event Hub

sqlloom exposes an HTTP server on `:8080` (or configured port):
- `/healthz`: JSON health check (uptime, memory, active connections, cache entries).
- `/metrics`: standard Prometheus metrics text format.
- `/api/stats`: JSON dashboard summary counters.
- `/api/tables`: active table list with cached query counts.
- `/api/cache/purge`: manual 1-click table or global cache flush endpoint.
- `/api/events`: Server-Sent Events (SSE) stream broadcasting every query event to the React 19 Cockpit.
