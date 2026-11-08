import { useState, useEffect, useCallback } from 'react';
import {
  Layers,
  Activity,
  Table as TableIcon,
  Terminal,
  ShieldAlert,
  History,
  Gauge,
} from 'lucide-react';
import { MetricCards } from './components/MetricCards';
import { QueryStream } from './components/QueryStream';
import { TableMap } from './components/TableMap';
import { QueryDetail } from './components/QueryDetail';
import { EvictionControl } from './components/EvictionControl';
import { RulesManager } from './components/RulesManager';
import { InvalidationView } from './components/InvalidationView';
import { SmokeBenchmarkModal } from './components/SmokeBenchmarkModal';
import {
  Stats,
  TableStat,
  QueryEvent,
  RulesSummary,
  InvalidationRecord,
  ChurnRecommendation,
} from './types';

export function App() {
  const [stats, setStats] = useState<Stats | null>(null);
  const [tables, setTables] = useState<TableStat[]>([]);
  const [events, setEvents] = useState<QueryEvent[]>([]);
  const [rules, setRules] = useState<RulesSummary | null>(null);
  const [invalidationHistory, setInvalidationHistory] = useState<InvalidationRecord[]>([]);
  const [churnRecs, setChurnRecs] = useState<ChurnRecommendation[]>([]);

  const [selectedQuery, setSelectedQuery] = useState<QueryEvent | null>(null);
  const [activeTab, setActiveTab] = useState<'stream' | 'tables' | 'rules' | 'invalidations'>('stream');
  const [connected, setConnected] = useState(false);
  const [isBenchmarkOpen, setIsBenchmarkOpen] = useState(false);

  // Fetch core stats & tables
  const fetchStats = useCallback(async () => {
    try {
      const [resStats, resTables] = await Promise.all([
        fetch('/api/stats'),
        fetch('/api/tables'),
      ]);
      if (resStats.ok) {
        const data = await resStats.json();
        setStats(data);
      }
      if (resTables.ok) {
        const data = await resTables.json();
        setTables(Array.isArray(data) ? data : []);
      }
    } catch {
      // Backend starting or offline
    }
  }, []);

  // Fetch rules & invalidations
  const fetchAdvancedData = useCallback(async () => {
    try {
      const [resRules, resInv, resChurn] = await Promise.all([
        fetch('/api/rules'),
        fetch('/api/invalidation/history'),
        fetch('/api/churn/recommendations'),
      ]);
      if (resRules.ok) {
        const data = await resRules.json();
        setRules(data);
      }
      if (resInv.ok) {
        const data = await resInv.json();
        setInvalidationHistory(Array.isArray(data) ? data : []);
      }
      if (resChurn.ok) {
        const data = await resChurn.json();
        setChurnRecs(Array.isArray(data) ? data : []);
      }
    } catch {
      // Ignore
    }
  }, []);

  // Fetch initial recent queries
  const fetchRecent = useCallback(async () => {
    try {
      const res = await fetch('/api/queries/recent');
      if (res.ok) {
        const data: QueryEvent[] = await res.json();
        if (Array.isArray(data)) {
          setEvents(data.reverse());
        }
      }
    } catch {
      // Ignore
    }
  }, []);

  useEffect(() => {
    fetchStats();
    fetchRecent();
    fetchAdvancedData();
    const interval = setInterval(() => {
      fetchStats();
      fetchAdvancedData();
    }, 2500);
    return () => clearInterval(interval);
  }, [fetchStats, fetchRecent, fetchAdvancedData]);

  // Subscribe to SSE real-time query stream
  useEffect(() => {
    const es = new EventSource('/api/events');

    es.onopen = () => {
      setConnected(true);
    };

    es.onmessage = (msg) => {
      try {
        const evt: QueryEvent = JSON.parse(msg.data);
        setEvents((prev) => [evt, ...prev.slice(0, 199)]);
        setStats((prev) => {
          if (!prev) return prev;
          const hits = evt.status === 'HIT' ? prev.cache_hits + 1 : prev.cache_hits;
          const misses = evt.status === 'MISS' ? prev.cache_misses + 1 : prev.cache_misses;
          const total = prev.total_queries + 1;
          const ratio = (hits / (hits + misses || 1)) * 100;
          return {
            ...prev,
            total_queries: total,
            cache_hits: hits,
            cache_misses: misses,
            hit_ratio_pct: ratio,
          };
        });
      } catch {
        // Ping
      }
    };

    es.onerror = () => {
      setConnected(false);
    };

    return () => {
      es.close();
    };
  }, []);

  const handlePurgeTable = async (tableName: string) => {
    await fetch('/api/cache/purge', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ table: tableName }),
    });
    await fetchStats();
    await fetchAdvancedData();
  };

  const handlePurgeAll = async (): Promise<number> => {
    const res = await fetch('/api/cache/purge', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ all: true }),
    });
    if (res.ok) {
      const data = await res.json();
      await fetchStats();
      await fetchAdvancedData();
      return data.evicted_count || 0;
    }
    return 0;
  };

  const handleExcludeTable = async (table: string) => {
    await fetch('/api/rules/exclude', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ table, exclude: true }),
    });
    await fetchAdvancedData();
  };

  return (
    <div className="min-h-screen flex flex-col font-sans">
      {/* Top Navigation */}
      <header className="border-b border-slate-800 bg-slate-950/80 backdrop-blur sticky top-0 z-40">
        <div className="max-w-7xl mx-auto px-4 sm:px-6 h-16 flex items-center justify-between">
          <div className="flex items-center space-x-3">
            <div className="w-8 h-8 rounded-lg bg-indigo-600 flex items-center justify-center shadow-lg shadow-indigo-600/30 text-white font-bold font-mono text-sm">
              <Layers className="w-4 h-4" />
            </div>
            <div>
              <div className="flex items-center space-x-2">
                <span className="font-bold text-white tracking-tight text-base">sqlloom</span>
                <span className="text-[10px] font-mono font-medium px-1.5 py-0.2 rounded bg-indigo-500/10 text-indigo-400 border border-indigo-500/20">
                  v1.1.0
                </span>
              </div>
              <p className="text-[11px] text-slate-400">
                Transparent PostgreSQL Wire-Protocol Query Cache
              </p>
            </div>
          </div>

          <div className="flex items-center space-x-3">
            {/* Smoke Benchmark Button */}
            <button
              onClick={() => setIsBenchmarkOpen(true)}
              className="flex items-center space-x-1.5 px-3 py-1.5 bg-indigo-600/90 hover:bg-indigo-500 text-white rounded-lg text-xs font-semibold shadow-sm transition-colors"
            >
              <Gauge className="w-3.5 h-3.5" />
              <span className="hidden sm:inline">Smoke Benchmark</span>
            </button>

            {/* Live SSE status indicator */}
            <div className="flex items-center space-x-2 text-xs font-mono bg-slate-900 border border-slate-800 px-2.5 py-1 rounded-lg">
              <span className="relative flex h-2 w-2">
                {connected && (
                  <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75" />
                )}
                <span
                  className={`relative inline-flex rounded-full h-2 w-2 ${
                    connected ? 'bg-emerald-500' : 'bg-rose-500'
                  }`}
                />
              </span>
              <span className="text-slate-300">
                {connected ? 'LIVE STREAM' : 'CONNECTING'}
              </span>
            </div>

            {/* Ports status pill */}
            <div className="hidden md:flex items-center space-x-1.5 text-xs font-mono text-slate-400 bg-slate-900 border border-slate-800 px-2.5 py-1 rounded-lg">
              <Terminal className="w-3.5 h-3.5 text-indigo-400" />
              <span>:5433</span>
              <span className="text-slate-600">→</span>
              <span className="text-slate-200">Upstream :5432</span>
            </div>

            <a
              href="/metrics"
              target="_blank"
              rel="noreferrer"
              className="text-xs font-mono text-indigo-400 hover:text-indigo-300 transition-colors hidden sm:inline"
            >
              /metrics
            </a>
          </div>
        </div>
      </header>

      {/* Main Container */}
      <main className="max-w-7xl mx-auto px-4 sm:px-6 py-6 flex-1 w-full space-y-6">
        {/* KPI Cards */}
        <MetricCards stats={stats} />

        {/* Action & Status banner */}
        <EvictionControl onPurgeAll={handlePurgeAll} />

        {/* Tab Switcher & Views */}
        <div className="space-y-4">
          <div className="flex border-b border-slate-800 space-x-6 text-xs font-semibold uppercase tracking-wider overflow-x-auto pb-px">
            <button
              onClick={() => setActiveTab('stream')}
              className={`pb-3 flex items-center space-x-2 transition-colors relative whitespace-nowrap ${
                activeTab === 'stream'
                  ? 'text-indigo-400'
                  : 'text-slate-400 hover:text-slate-200'
              }`}
            >
              <Activity className="w-4 h-4" />
              <span>Query Stream</span>
              {activeTab === 'stream' && (
                <div className="absolute bottom-0 left-0 right-0 h-0.5 bg-indigo-500 rounded-full" />
              )}
            </button>

            <button
              onClick={() => setActiveTab('tables')}
              className={`pb-3 flex items-center space-x-2 transition-colors relative whitespace-nowrap ${
                activeTab === 'tables'
                  ? 'text-indigo-400'
                  : 'text-slate-400 hover:text-slate-200'
              }`}
            >
              <TableIcon className="w-4 h-4" />
              <span>Table Map ({(tables || []).length})</span>
              {activeTab === 'tables' && (
                <div className="absolute bottom-0 left-0 right-0 h-0.5 bg-indigo-500 rounded-full" />
              )}
            </button>

            <button
              onClick={() => setActiveTab('rules')}
              className={`pb-3 flex items-center space-x-2 transition-colors relative whitespace-nowrap ${
                activeTab === 'rules'
                  ? 'text-indigo-400'
                  : 'text-slate-400 hover:text-slate-200'
              }`}
            >
              <ShieldAlert className="w-4 h-4" />
              <span>Rules & Policies</span>
              {activeTab === 'rules' && (
                <div className="absolute bottom-0 left-0 right-0 h-0.5 bg-indigo-500 rounded-full" />
              )}
            </button>

            <button
              onClick={() => setActiveTab('invalidations')}
              className={`pb-3 flex items-center space-x-2 transition-colors relative whitespace-nowrap ${
                activeTab === 'invalidations'
                  ? 'text-indigo-400'
                  : 'text-slate-400 hover:text-slate-200'
              }`}
            >
              <History className="w-4 h-4" />
              <span>Invalidations & Churn</span>
              {(churnRecs || []).length > 0 && (
                <span className="w-2 h-2 rounded-full bg-amber-400" />
              )}
              {activeTab === 'invalidations' && (
                <div className="absolute bottom-0 left-0 right-0 h-0.5 bg-indigo-500 rounded-full" />
              )}
            </button>
          </div>

          {activeTab === 'stream' && (
            <QueryStream events={events} onSelectQuery={setSelectedQuery} />
          )}

          {activeTab === 'tables' && (
            <TableMap tables={tables} onPurgeTable={handlePurgeTable} />
          )}

          {activeTab === 'rules' && (
            <RulesManager rules={rules} onRefresh={fetchAdvancedData} />
          )}

          {activeTab === 'invalidations' && (
            <InvalidationView
              history={invalidationHistory}
              recommendations={churnRecs}
              onExcludeTable={handleExcludeTable}
            />
          )}
        </div>
      </main>

      {/* Query Detail Modal */}
      <QueryDetail query={selectedQuery} onClose={() => setSelectedQuery(null)} />

      {/* Smoke Benchmark Modal */}
      <SmokeBenchmarkModal
        isOpen={isBenchmarkOpen}
        onClose={() => setIsBenchmarkOpen(false)}
      />

      {/* Footer */}
      <footer className="border-t border-slate-800/80 py-4 text-center text-xs text-slate-400 font-mono">
        <p>sqlloom &bull; Transparent PostgreSQL Wire-Protocol Query Cache &bull; Built in Go & React 19</p>
      </footer>
    </div>
  );
}
