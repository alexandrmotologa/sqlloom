import React from 'react';
import { Zap, Database, Clock, HardDrive, Activity, ShieldCheck } from 'lucide-react';
import { Stats } from '../types';

interface MetricCardsProps {
  stats: Stats | null;
}

export const MetricCards: React.FC<MetricCardsProps> = ({ stats }) => {
  const hitRatio = stats?.hit_ratio_pct ?? 0;
  const total = stats?.total_queries ?? 0;
  const hits = stats?.cache_hits ?? 0;
  const misses = stats?.cache_misses ?? 0;
  const savedMs = stats?.saved_time_ms ?? 0;
  const memBytes = stats?.memory_bytes ?? 0;
  const memMb = (memBytes / (1024 * 1024)).toFixed(2);
  const entries = stats?.entries ?? 0;
  const conns = stats?.active_conns ?? 0;

  const formatSavedTime = (ms: number) => {
    if (ms >= 60000) return `${(ms / 60000).toFixed(1)} min`;
    if (ms >= 1000) return `${(ms / 1000).toFixed(2)} s`;
    return `${ms.toFixed(1)} ms`;
  };

  return (
    <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-6 gap-4">
      {/* Hit Ratio */}
      <div className="bg-slate-900/90 border border-slate-800 rounded-xl p-4 shadow-sm relative overflow-hidden group">
        <div className="flex items-center justify-between text-slate-400 mb-2">
          <span className="text-xs font-medium uppercase tracking-wider">Hit Ratio</span>
          <Zap className="w-4 h-4 text-emerald-400" />
        </div>
        <div className="flex items-baseline space-x-2">
          <span className="text-2xl font-bold tracking-tight text-white">
            {hitRatio.toFixed(1)}%
          </span>
          <span className="text-xs text-slate-400">of queries</span>
        </div>
        <div className="w-full bg-slate-800 h-1.5 rounded-full mt-3 overflow-hidden">
          <div
            className="bg-emerald-500 h-full rounded-full transition-all duration-500"
            style={{ width: `${Math.min(100, Math.max(0, hitRatio))}%` }}
          />
        </div>
      </div>

      {/* Total Queries */}
      <div className="bg-slate-900/90 border border-slate-800 rounded-xl p-4 shadow-sm relative overflow-hidden">
        <div className="flex items-center justify-between text-slate-400 mb-2">
          <span className="text-xs font-medium uppercase tracking-wider">Total Traffic</span>
          <Activity className="w-4 h-4 text-indigo-400" />
        </div>
        <div className="flex items-baseline space-x-2">
          <span className="text-2xl font-bold tracking-tight text-white">
            {total.toLocaleString()}
          </span>
          <span className="text-xs text-slate-400">statements</span>
        </div>
        <div className="text-xs text-slate-400 mt-3 flex items-center justify-between">
          <span>Hits: <strong className="text-emerald-400">{hits}</strong></span>
          <span>Misses: <strong className="text-blue-400">{misses}</strong></span>
        </div>
      </div>

      {/* Latency Saved */}
      <div className="bg-slate-900/90 border border-slate-800 rounded-xl p-4 shadow-sm relative overflow-hidden">
        <div className="flex items-center justify-between text-slate-400 mb-2">
          <span className="text-xs font-medium uppercase tracking-wider">Saved DB Time</span>
          <Clock className="w-4 h-4 text-amber-400" />
        </div>
        <div className="flex items-baseline space-x-2">
          <span className="text-2xl font-bold tracking-tight text-amber-300">
            {formatSavedTime(savedMs)}
          </span>
        </div>
        <p className="text-xs text-slate-400 mt-3 truncate">
          Accelerated reads (&lt; 0.5ms)
        </p>
      </div>

      {/* Memory Footprint */}
      <div className="bg-slate-900/90 border border-slate-800 rounded-xl p-4 shadow-sm relative overflow-hidden">
        <div className="flex items-center justify-between text-slate-400 mb-2">
          <span className="text-xs font-medium uppercase tracking-wider">Memory RAM</span>
          <HardDrive className="w-4 h-4 text-cyan-400" />
        </div>
        <div className="flex items-baseline space-x-2">
          <span className="text-2xl font-bold tracking-tight text-white">
            {memMb}
          </span>
          <span className="text-xs text-slate-400">/ 32 MB</span>
        </div>
        <div className="w-full bg-slate-800 h-1.5 rounded-full mt-3 overflow-hidden">
          <div
            className="bg-cyan-500 h-full rounded-full transition-all duration-500"
            style={{ width: `${Math.min(100, (parseFloat(memMb) / 32) * 100)}%` }}
          />
        </div>
      </div>

      {/* Cached Items */}
      <div className="bg-slate-900/90 border border-slate-800 rounded-xl p-4 shadow-sm relative overflow-hidden">
        <div className="flex items-center justify-between text-slate-400 mb-2">
          <span className="text-xs font-medium uppercase tracking-wider">Cached Queries</span>
          <Database className="w-4 h-4 text-purple-400" />
        </div>
        <div className="flex items-baseline space-x-2">
          <span className="text-2xl font-bold tracking-tight text-white">
            {entries.toLocaleString()}
          </span>
          <span className="text-xs text-slate-400">entries</span>
        </div>
        <p className="text-xs text-slate-400 mt-3 truncate">
          LRU striped hash map
        </p>
      </div>

      {/* Active Connections */}
      <div className="bg-slate-900/90 border border-slate-800 rounded-xl p-4 shadow-sm relative overflow-hidden">
        <div className="flex items-center justify-between text-slate-400 mb-2">
          <span className="text-xs font-medium uppercase tracking-wider">Clients</span>
          <ShieldCheck className="w-4 h-4 text-emerald-400" />
        </div>
        <div className="flex items-baseline space-x-2">
          <span className="text-2xl font-bold tracking-tight text-white">
            {conns}
          </span>
          <span className="text-xs text-emerald-400 font-medium">active</span>
        </div>
        <p className="text-xs text-slate-400 mt-3 truncate">
          Wire protocol v3.0
        </p>
      </div>
    </div>
  );
};
