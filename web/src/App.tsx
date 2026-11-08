import { useState, useEffect, useCallback } from 'react';
import { Layers, Activity, Table as TableIcon, Terminal } from 'lucide-react';
import { MetricCards } from './components/MetricCards';
import { QueryStream } from './components/QueryStream';
import { TableMap } from './components/TableMap';
import { QueryDetail } from './components/QueryDetail';
import { EvictionControl } from './components/EvictionControl';
import { Stats, TableStat, QueryEvent } from './types';

export function App() {
  const [stats, setStats] = useState<Stats | null>(null);
  const [tables, setTables] = useState<TableStat[]>([]);
  const [events, setEvents] = useState<QueryEvent[]>([]);
  const [selectedQuery, setSelectedQuery] = useState<QueryEvent | null>(null);
  const [activeTab, setActiveTab] = useState<'stream' | 'tables'>('stream');
  const [connected, setConnected] = useState(false);

  // Fetch stats and tables
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
        setTables(data);
      }
    } catch {
      // Backend may be starting or offline
    }
  }, []);

  // Fetch initial recent queries
  const fetchRecent = useCallback(async () => {
    try {
      const res = await fetch('/api/queries/recent');
      if (res.ok) {
        const data: QueryEvent[] = await res.json();
        if (Array.isArray(data)) {
          // Reverse so newest is first
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
    const interval = setInterval(fetchStats, 2000);
    return () => clearInterval(interval);
  }, [fetchStats, fetchRecent]);

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
        // Incrementally update hits/misses
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
        // Ping or non-json frame
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
      return data.evicted_count || 0;
    }
    return 0;
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
                  v1.0.0
                </span>
              </div>
              <p className="text-[11px] text-slate-400">
                Transparent PostgreSQL Wire-Protocol Query Cache
              </p>
            </div>
          </div>

          <div className="flex items-center space-x-4">
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
              className="text-xs font-mono text-indigo-400 hover:text-indigo-300 transition-colors"
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
          <div className="flex border-b border-slate-800 space-x-6 text-xs font-semibold uppercase tracking-wider">
            <button
              onClick={() => setActiveTab('stream')}
              className={`pb-3 flex items-center space-x-2 transition-colors relative ${
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
              className={`pb-3 flex items-center space-x-2 transition-colors relative ${
                activeTab === 'tables'
                  ? 'text-indigo-400'
                  : 'text-slate-400 hover:text-slate-200'
              }`}
            >
              <TableIcon className="w-4 h-4" />
              <span>Table Dependency Map ({tables.length})</span>
              {activeTab === 'tables' && (
                <div className="absolute bottom-0 left-0 right-0 h-0.5 bg-indigo-500 rounded-full" />
              )}
            </button>
          </div>

          {activeTab === 'stream' ? (
            <QueryStream events={events} onSelectQuery={setSelectedQuery} />
          ) : (
            <TableMap tables={tables} onPurgeTable={handlePurgeTable} />
          )}
        </div>
      </main>

      {/* Query Detail Modal */}
      <QueryDetail query={selectedQuery} onClose={() => setSelectedQuery(null)} />

      {/* Footer */}
      <footer className="border-t border-slate-800/80 py-4 text-center text-xs text-slate-400 font-mono">
        <p>sqlloom &bull; Transparent PostgreSQL Wire-Protocol Query Cache &bull; Built in Go & React 19</p>
      </footer>
    </div>
  );
}
