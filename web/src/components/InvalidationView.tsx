import React from 'react';
import { AlertTriangle, History, ArrowDownRight, ShieldBan } from 'lucide-react';
import { InvalidationRecord, ChurnRecommendation } from '../types';

interface InvalidationViewProps {
  history: InvalidationRecord[];
  recommendations: ChurnRecommendation[];
  onExcludeTable: (table: string) => Promise<void>;
}

export const InvalidationView: React.FC<InvalidationViewProps> = ({
  history,
  recommendations,
  onExcludeTable,
}) => {
  return (
    <div className="space-y-6">
      {/* Churn Recommendations */}
      {recommendations.length > 0 && (
        <div className="space-y-3">
          <div className="flex items-center space-x-2 text-xs font-semibold uppercase tracking-wider text-amber-400">
            <AlertTriangle className="w-4 h-4" />
            <span>Cache Churn & Invalidation Warnings</span>
          </div>

          <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
            {recommendations.map((rec) => (
              <div
                key={rec.table}
                className="bg-amber-950/20 border border-amber-900/40 rounded-xl p-4 flex flex-col justify-between space-y-3"
              >
                <div>
                  <div className="flex items-center justify-between mb-1">
                    <span className="text-xs font-mono font-bold text-amber-300">
                      table: {rec.table}
                    </span>
                    <span
                      className={`text-[10px] px-1.5 py-0.5 rounded font-mono font-semibold ${
                        rec.severity === 'HIGH'
                          ? 'bg-rose-500/20 text-rose-400 border border-rose-500/30'
                          : 'bg-amber-500/20 text-amber-400 border border-amber-500/30'
                      }`}
                    >
                      {rec.severity} CHURN
                    </span>
                  </div>
                  <p className="text-xs text-slate-300">
                    {rec.message}
                  </p>
                </div>

                <div className="flex justify-end pt-1">
                  <button
                    onClick={() => onExcludeTable(rec.table)}
                    className="flex items-center space-x-1.5 px-3 py-1 bg-amber-600 hover:bg-amber-500 text-white rounded text-xs font-medium transition-colors"
                  >
                    <ShieldBan className="w-3.5 h-3.5" />
                    <span>Exclude '{rec.table}' from Cache</span>
                  </button>
                </div>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Invalidation History Table */}
      <div className="bg-slate-900/90 border border-slate-800 rounded-xl overflow-hidden shadow-sm flex flex-col h-[520px]">
        <div className="p-4 border-b border-slate-800 flex items-center justify-between">
          <div className="flex items-center space-x-2">
            <History className="w-4 h-4 text-purple-400" />
            <h2 className="text-sm font-semibold text-white tracking-wide uppercase">
              Table Invalidation & Write Activity Log
            </h2>
          </div>
          <span className="text-xs text-slate-400 font-mono">
            {history.length} events logged
          </span>
        </div>

        <div className="flex-1 overflow-y-auto divide-y divide-slate-800/60 font-mono text-xs">
          {history.length === 0 ? (
            <div className="p-12 text-center text-slate-500">
              No write invalidations recorded yet. Writes to tables (INSERT, UPDATE, DELETE) will log here with eviction impact.
            </div>
          ) : (
            history.map((rec) => (
              <div
                key={rec.id}
                className="p-3.5 hover:bg-slate-800/30 transition-colors flex items-center justify-between gap-4"
              >
                <div className="flex items-center space-x-3 min-w-0">
                  <span className="px-2 py-0.5 rounded text-[10px] font-bold bg-purple-500/10 text-purple-400 border border-purple-500/20">
                    {rec.operation}
                  </span>
                  <div className="flex items-center space-x-2">
                    <span className="text-white font-semibold">
                      {rec.table}
                    </span>
                    <span className="text-slate-500 text-[11px]">→</span>
                    <span className="text-rose-400 bg-rose-950/40 px-1.5 py-0.2 rounded border border-rose-900/40 text-[10px]">
                      <ArrowDownRight className="w-3 h-3 inline mr-0.5" />
                      {rec.evicted_count} queries purged
                    </span>
                  </div>
                </div>

                <div className="flex items-center space-x-4 shrink-0 text-slate-400 text-right">
                  <span className="text-[11px]">
                    {rec.duration_ms.toFixed(2)} ms
                  </span>
                  <span className="text-[11px] w-20">
                    {new Date(rec.timestamp).toLocaleTimeString([], {
                      hour: '2-digit',
                      minute: '2-digit',
                      second: '2-digit',
                    })}
                  </span>
                </div>
              </div>
            ))
          )}
        </div>
      </div>
    </div>
  );
};
