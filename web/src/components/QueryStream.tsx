import React, { useState } from 'react';
import { Search, Zap, ArrowRight, ShieldAlert, RefreshCw } from 'lucide-react';
import { QueryEvent, QueryStatus } from '../types';

interface QueryStreamProps {
  events: QueryEvent[];
  onSelectQuery: (event: QueryEvent) => void;
}

export const QueryStream: React.FC<QueryStreamProps> = ({ events, onSelectQuery }) => {
  const [filter, setFilter] = useState<'ALL' | QueryStatus>('ALL');
  const [search, setSearch] = useState('');

  const filteredEvents = events.filter((e) => {
    if (filter !== 'ALL' && e.status !== filter) return false;
    if (search.trim()) {
      const q = search.toLowerCase();
      const matchSQL = e.sql.toLowerCase().includes(q);
      const matchTable = e.tables.some((t) => t.toLowerCase().includes(q));
      if (!matchSQL && !matchTable) return false;
    }
    return true;
  });

  const getStatusBadge = (status: QueryStatus) => {
    switch (status) {
      case 'HIT':
        return (
          <span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-semibold bg-emerald-500/10 text-emerald-400 border border-emerald-500/20">
            <Zap className="w-3 h-3 mr-1 text-emerald-400" />
            CACHE HIT
          </span>
        );
      case 'MISS':
        return (
          <span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-semibold bg-blue-500/10 text-blue-400 border border-blue-500/20">
            <ArrowRight className="w-3 h-3 mr-1 text-blue-400" />
            CACHE MISS
          </span>
        );
      case 'BYPASS':
        return (
          <span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-semibold bg-amber-500/10 text-amber-400 border border-amber-500/20">
            <ShieldAlert className="w-3 h-3 mr-1 text-amber-400" />
            BYPASS
          </span>
        );
      case 'INVALIDATE':
        return (
          <span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-semibold bg-purple-500/10 text-purple-400 border border-purple-500/20">
            <RefreshCw className="w-3 h-3 mr-1 text-purple-400" />
            INVALIDATE
          </span>
        );
    }
  };

  const formatDuration = (ms: number, status: QueryStatus) => {
    if (status === 'HIT') {
      return (
        <span className="text-emerald-400 font-mono font-bold text-xs bg-emerald-950/60 px-1.5 py-0.5 rounded border border-emerald-800/40">
          {ms.toFixed(2)} ms
        </span>
      );
    }
    return (
      <span className="text-slate-400 font-mono text-xs">
        {ms.toFixed(2)} ms
      </span>
    );
  };

  return (
    <div className="bg-slate-900/90 border border-slate-800 rounded-xl overflow-hidden shadow-sm flex flex-col h-[560px]">
      {/* Header & Controls */}
      <div className="p-4 border-b border-slate-800 flex flex-col sm:flex-row sm:items-center justify-between gap-3">
        <div className="flex items-center space-x-2">
          <h2 className="text-sm font-semibold text-white tracking-wide uppercase">
            Live Query Stream
          </h2>
          <span className="text-xs bg-slate-800 text-slate-400 px-2 py-0.5 rounded-full font-mono">
            {filteredEvents.length} events
          </span>
        </div>

        <div className="flex flex-wrap items-center gap-2">
          {/* Search bar */}
          <div className="relative">
            <Search className="w-3.5 h-3.5 text-slate-500 absolute left-2.5 top-1/2 -translate-y-1/2" />
            <input
              type="text"
              placeholder="Filter SQL or table..."
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              className="pl-8 pr-3 py-1 bg-slate-950 border border-slate-800 rounded-lg text-xs text-slate-200 placeholder-slate-500 focus:outline-none focus:border-indigo-500 w-48 sm:w-60"
            />
          </div>

          {/* Status Filter Tabs */}
          <div className="flex bg-slate-950 p-0.5 rounded-lg border border-slate-800 text-xs">
            {(['ALL', 'HIT', 'MISS', 'BYPASS', 'INVALIDATE'] as const).map((tab) => (
              <button
                key={tab}
                onClick={() => setFilter(tab)}
                className={`px-2.5 py-1 rounded-md transition-colors font-medium ${
                  filter === tab
                    ? 'bg-slate-800 text-white shadow-sm'
                    : 'text-slate-400 hover:text-slate-200'
                }`}
              >
                {tab}
              </button>
            ))}
          </div>
        </div>
      </div>

      {/* Stream list */}
      <div className="flex-1 overflow-y-auto divide-y divide-slate-800/60 font-mono text-xs">
        {filteredEvents.length === 0 ? (
          <div className="p-12 text-center text-slate-500">
            No queries matching filter criteria. Stream is listening on port 5433...
          </div>
        ) : (
          filteredEvents.map((evt) => (
            <div
              key={evt.id}
              onClick={() => onSelectQuery(evt)}
              className="p-3 hover:bg-slate-800/40 transition-colors cursor-pointer flex items-center justify-between gap-4 group"
            >
              <div className="flex items-center space-x-3 min-w-0 flex-1">
                <div className="shrink-0">{getStatusBadge(evt.status)}</div>

                <div className="min-w-0 flex-1">
                  <p className="text-slate-200 truncate font-mono text-xs group-hover:text-indigo-300 transition-colors">
                    {evt.sql}
                  </p>
                  <div className="flex items-center space-x-2 mt-1">
                    {evt.tables.map((t) => (
                      <span
                        key={t}
                        className="text-[10px] bg-slate-800/80 text-slate-400 px-1.5 py-0.2 rounded"
                      >
                        {t}
                      </span>
                    ))}
                    {evt.reason && (
                      <span className="text-[10px] text-amber-400/80 italic">
                        ({evt.reason})
                      </span>
                    )}
                  </div>
                </div>
              </div>

              <div className="flex items-center space-x-4 shrink-0 text-right">
                <div>{formatDuration(evt.duration_ms, evt.status)}</div>
                <div className="text-[11px] text-slate-400 w-16">
                  {new Date(evt.timestamp).toLocaleTimeString([], {
                    hour: '2-digit',
                    minute: '2-digit',
                    second: '2-digit',
                  })}
                </div>
              </div>
            </div>
          ))
        )}
      </div>
    </div>
  );
};
