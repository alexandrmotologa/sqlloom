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

export interface PatternRule {
  id: string;
  pattern: string;
  action: 'BYPASS' | 'CACHE';
  ttl_sec?: number;
}

export interface RulesSummary {
  excluded_tables: string[];
  table_ttls: Record<string, number>;
  patterns: PatternRule[];
}

export interface InvalidationRecord {
  id: string;
  table: string;
  operation: string;
  evicted_count: number;
  duration_ms: number;
  timestamp: string;
}

export interface ChurnRecommendation {
  table: string;
  writes_per_min: number;
  severity: 'HIGH' | 'MEDIUM';
  message: string;
}

export interface BenchmarkResult {
  total_queries: number;
  duration_ms: number;
  throughput_qps: number;
  cache_hits: number;
  cache_misses: number;
  hit_ratio_pct: number;
  p50_latency_ms: number;
  p99_latency_ms: number;
  min_latency_ms: number;
  max_latency_ms: number;
}
