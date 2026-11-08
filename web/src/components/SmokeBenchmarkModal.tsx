import React, { useState } from 'react';
import { X, Play, RefreshCw, Zap, Gauge, Clock, ShieldCheck } from 'lucide-react';
import { BenchmarkResult } from '../types';

interface SmokeBenchmarkModalProps {
  isOpen: boolean;
  onClose: () => void;
}

export const SmokeBenchmarkModal: React.FC<SmokeBenchmarkModalProps> = ({ isOpen, onClose }) => {
  const [totalQueries, setTotalQueries] = useState(400);
  const [concurrency, setConcurrency] = useState(8);
  const [writeRatio, setWriteRatio] = useState(0.1);
  const [table, setTable] = useState('users');
  const [database, setDatabase] = useState('');
  const [running, setRunning] = useState(false);
  const [result, setResult] = useState<BenchmarkResult | null>(null);
  const [error, setError] = useState<string | null>(null);

  if (!isOpen) return null;

  const handleRun = async () => {
    setRunning(true);
    setError(null);
    setResult(null);

    try {
      const res = await fetch('/api/benchmark/run', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          total_queries: totalQueries,
          concurrency: concurrency,
          write_ratio: writeRatio,
          table: table.trim() || undefined,
          database: database.trim() || undefined,
        }),
      });

      if (!res.ok) {
        const text = await res.text();
        throw new Error(text || 'Benchmark execution failed');
      }

      const data: BenchmarkResult = await res.json();
      setResult(data);
    } catch (err: unknown) {
      if (err instanceof Error) {
        setError(err.message);
      } else {
        setError('Unknown error during benchmark');
      }
    } finally {
      setRunning(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/60 backdrop-blur-sm animate-in fade-in duration-200">
      <div className="bg-slate-900 border border-slate-800 rounded-xl max-w-xl w-full p-6 shadow-2xl relative flex flex-col">
        {/* Header */}
        <div className="flex items-center justify-between pb-4 border-b border-slate-800">
          <div className="flex items-center space-x-2.5">
            <div className="w-8 h-8 rounded-lg bg-indigo-600/20 border border-indigo-500/30 flex items-center justify-center text-indigo-400">
              <Gauge className="w-4 h-4" />
            </div>
            <div>
              <h2 className="text-sm font-bold text-white uppercase tracking-wider">
                Smoke Benchmark & Load Simulator
              </h2>
              <p className="text-[11px] text-slate-400">
                Fire concurrent queries through sqlloom to measure latency and hit ratios live.
              </p>
            </div>
          </div>

          <button
            onClick={onClose}
            className="text-slate-400 hover:text-white p-1 rounded-lg hover:bg-slate-800 transition-colors"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        {/* Content */}
        <div className="py-5 space-y-5">
          {/* Controls */}
          <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
            <div>
              <label className="text-xs text-slate-400 block mb-1">
                Total Queries
              </label>
              <input
                type="number"
                min="50"
                max="2000"
                step="50"
                value={totalQueries}
                onChange={(e) => setTotalQueries(parseInt(e.target.value, 10) || 100)}
                className="w-full bg-slate-950 border border-slate-800 rounded-lg px-3 py-1.5 text-xs text-white focus:outline-none focus:border-indigo-500 font-mono"
              />
            </div>

            <div>
              <label className="text-xs text-slate-400 block mb-1">
                Concurrency (Workers)
              </label>
              <input
                type="number"
                min="1"
                max="20"
                value={concurrency}
                onChange={(e) => setConcurrency(parseInt(e.target.value, 10) || 1)}
                className="w-full bg-slate-950 border border-slate-800 rounded-lg px-3 py-1.5 text-xs text-white focus:outline-none focus:border-indigo-500 font-mono"
              />
            </div>

            <div>
              <label className="text-xs text-slate-400 block mb-1">
                Write Ratio (Updates)
              </label>
              <select
                value={writeRatio}
                onChange={(e) => setWriteRatio(parseFloat(e.target.value))}
                className="w-full bg-slate-950 border border-slate-800 rounded-lg px-3 py-1.5 text-xs text-white focus:outline-none focus:border-indigo-500 font-mono"
              >
                <option value={0.0}>0% (Pure Reads)</option>
                <option value={0.05}>5% Writes</option>
                <option value={0.1}>10% Writes</option>
                <option value={0.25}>25% Writes</option>
              </select>
            </div>
          </div>

          <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <div>
              <label className="text-xs text-slate-400 block mb-1">
                Target Table
              </label>
              <input
                type="text"
                placeholder="users (default)"
                value={table}
                onChange={(e) => setTable(e.target.value)}
                className="w-full bg-slate-950 border border-slate-800 rounded-lg px-3 py-1.5 text-xs text-white focus:outline-none focus:border-indigo-500 font-mono"
              />
            </div>

            <div>
              <label className="text-xs text-slate-400 block mb-1">
                Database (Optional)
              </label>
              <input
                type="text"
                placeholder="postgres (default)"
                value={database}
                onChange={(e) => setDatabase(e.target.value)}
                className="w-full bg-slate-950 border border-slate-800 rounded-lg px-3 py-1.5 text-xs text-white focus:outline-none focus:border-indigo-500 font-mono"
              />
            </div>
          </div>

          {/* Trigger button */}
          <div className="flex justify-center">
            <button
              onClick={handleRun}
              disabled={running}
              className="px-6 py-2 bg-indigo-600 hover:bg-indigo-500 text-white rounded-lg text-xs font-bold uppercase tracking-wider flex items-center space-x-2 transition-all shadow-lg shadow-indigo-600/30 disabled:opacity-50"
            >
              {running ? (
                <>
                  <RefreshCw className="w-4 h-4 animate-spin" />
                  <span>Executing {totalQueries} Queries...</span>
                </>
              ) : (
                <>
                  <Play className="w-4 h-4" />
                  <span>Execute Smoke Benchmark</span>
                </>
              )}
            </button>
          </div>

          {error && (
            <div className="p-3 bg-rose-950/30 border border-rose-900/50 rounded-lg text-rose-300 text-xs">
              {error}
            </div>
          )}

          {/* Result Card */}
          {result && (
            <div className="bg-slate-950 border border-slate-800 rounded-xl p-4 space-y-4 animate-in fade-in">
              <div className="flex items-center justify-between pb-3 border-b border-slate-800/80">
                <span className="text-xs font-bold text-white uppercase tracking-wider flex items-center gap-1.5">
                  <ShieldCheck className="w-4 h-4 text-emerald-400" />
                  Benchmark Execution Completed
                </span>
                <span className="text-xs text-slate-400 font-mono">
                  {result.duration_ms.toFixed(1)} ms elapsed
                </span>
              </div>

              <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
                <div className="bg-slate-900/80 p-3 rounded-lg border border-slate-800/80 text-center">
                  <span className="text-[10px] text-slate-400 uppercase font-semibold block">Throughput</span>
                  <span className="text-lg font-bold font-mono text-indigo-400">
                    {Math.round(result.throughput_qps).toLocaleString()}
                  </span>
                  <span className="text-[10px] text-slate-500 block">QPS</span>
                </div>

                <div className="bg-slate-900/80 p-3 rounded-lg border border-slate-800/80 text-center">
                  <span className="text-[10px] text-slate-400 uppercase font-semibold block">Hit Ratio</span>
                  <span className="text-lg font-bold font-mono text-emerald-400">
                    {result.hit_ratio_pct.toFixed(1)}%
                  </span>
                  <span className="text-[10px] text-slate-500 block">
                    {result.cache_hits} hits / {result.cache_misses} miss
                  </span>
                </div>

                <div className="bg-slate-900/80 p-3 rounded-lg border border-slate-800/80 text-center">
                  <span className="text-[10px] text-slate-400 uppercase font-semibold block flex items-center justify-center gap-1">
                    <Clock className="w-3 h-3" /> p50 Latency
                  </span>
                  <span className="text-lg font-bold font-mono text-emerald-300">
                    {result.p50_latency_ms.toFixed(2)} ms
                  </span>
                  <span className="text-[10px] text-slate-500 block">&lt; 0.5ms Read</span>
                </div>

                <div className="bg-slate-900/80 p-3 rounded-lg border border-slate-800/80 text-center">
                  <span className="text-[10px] text-slate-400 uppercase font-semibold block flex items-center justify-center gap-1">
                    <Zap className="w-3 h-3 text-amber-400" /> p99 Latency
                  </span>
                  <span className="text-lg font-bold font-mono text-amber-300">
                    {result.p99_latency_ms.toFixed(2)} ms
                  </span>
                  <span className="text-[10px] text-slate-500 block">Peak Bound</span>
                </div>
              </div>
            </div>
          )}
        </div>

        {/* Footer */}
        <div className="pt-3 border-t border-slate-800 flex justify-end">
          <button
            onClick={onClose}
            className="px-4 py-1.5 bg-slate-800 hover:bg-slate-700 text-white rounded-lg text-xs font-medium transition-colors"
          >
            Close
          </button>
        </div>
      </div>
    </div>
  );
};
