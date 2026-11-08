export type QueryStatus = 'HIT' | 'MISS' | 'BYPASS' | 'INVALIDATE';

export interface QueryEvent {
  id: string;
  timestamp: string;
  sql: string;
  normalized: string;
  status: QueryStatus;
  duration_ms: number;
  tables: string[];
  reason?: string;
}

export interface Stats {
  total_queries: number;
  cache_hits: number;
  cache_misses: number;
  bypasses: number;
  hit_ratio_pct: number;
  saved_time_ms: number;
  invalidations: number;
  memory_bytes: number;
  entries: number;
  active_conns: number;
}

export interface TableStat {
  name: string;
  cached_queries: number;
}

export interface HealthStatus {
  status: string;
  uptime_sec: number;
  memory_mb: number;
  entries: number;
  connections: number;
}
