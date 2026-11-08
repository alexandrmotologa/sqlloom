import React from 'react';
import { X, Zap, Clock, Database, CheckCircle, ShieldCheck } from 'lucide-react';
import { QueryEvent } from '../types';

interface QueryDetailProps {
  query: QueryEvent | null;
  onClose: () => void;
}

export const QueryDetail: React.FC<QueryDetailProps> = ({ query, onClose }) => {
  if (!query) return null;

  const isHit = query.status === 'HIT';
  const baselineMs = 15.0; // Typical database roundtrip
  const speedup = isHit && query.duration_ms > 0 ? (baselineMs / query.duration_ms).toFixed(0) : null;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/60 backdrop-blur-sm animate-in fade-in duration-200">
      <div className="bg-slate-900 border border-slate-800 rounded-xl max-w-2xl w-full p-6 shadow-2xl relative flex flex-col max-h-[85vh]">
        {/* Header */}
        <div className="flex items-center justify-between pb-4 border-b border-slate-800">
          <div className="flex items-center space-x-3">
            <span
              className={`px-2.5 py-1 rounded text-xs font-bold font-mono ${
                isHit
                  ? 'bg-emerald-500/10 text-emerald-400 border border-emerald-500/20'
                  : 'bg-blue-500/10 text-blue-400 border border-blue-500/20'
              }`}
            >
              {query.status}
            </span>
            <span className="text-xs text-slate-400 font-mono">{query.id}</span>
          </div>

          <button
            onClick={onClose}
            className="text-slate-400 hover:text-white p-1 rounded-lg hover:bg-slate-800 transition-colors"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        {/* Scrollable Content */}
        <div className="flex-1 overflow-y-auto py-4 space-y-5">
          {/* Latency Comparison Card */}
          <div className="grid grid-cols-2 gap-3">
            <div className="bg-slate-950 p-3 rounded-lg border border-slate-800">
              <span className="text-xs text-slate-400 flex items-center gap-1.5 mb-1">
                <Clock className="w-3.5 h-3.5 text-slate-400" />
                Execution Duration
              </span>
              <div className="flex items-baseline gap-2">
                <span className={`text-xl font-bold font-mono ${isHit ? 'text-emerald-400' : 'text-slate-200'}`}>
                  {query.duration_ms.toFixed(3)} ms
                </span>
                {isHit && (
                  <span className="text-xs text-emerald-500 font-semibold flex items-center gap-1">
                    <Zap className="w-3 h-3" /> Sub-0.5ms Read
                  </span>
                )}
              </div>
            </div>

            <div className="bg-slate-950 p-3 rounded-lg border border-slate-800">
              <span className="text-xs text-slate-400 flex items-center gap-1.5 mb-1">
                <ShieldCheck className="w-3.5 h-3.5 text-indigo-400" />
                Acceleration Factor
              </span>
              <div className="text-xl font-bold font-mono text-indigo-300">
                {speedup ? `${speedup}x Faster` : 'Direct Upstream PG'}
              </div>
            </div>
          </div>

          {/* Raw SQL Statement */}
          <div>
            <label className="text-xs font-semibold uppercase tracking-wider text-slate-400 mb-1.5 block">
              Raw SQL Statement
            </label>
            <div className="bg-slate-950 p-3 rounded-lg border border-slate-800 text-xs font-mono text-emerald-300 overflow-x-auto whitespace-pre-wrap selection:bg-emerald-950">
              {query.sql}
            </div>
          </div>

          {/* Normalized Template */}
          {query.normalized && (
            <div>
              <label className="text-xs font-semibold uppercase tracking-wider text-slate-400 mb-1.5 block">
                Normalized Fingerprint Template
              </label>
              <div className="bg-slate-950 p-3 rounded-lg border border-slate-800 text-xs font-mono text-indigo-300 overflow-x-auto whitespace-pre-wrap">
                {query.normalized}
              </div>
            </div>
          )}

          {/* Tables Touched */}
          <div>
            <label className="text-xs font-semibold uppercase tracking-wider text-slate-400 mb-1.5 block flex items-center gap-1">
              <Database className="w-3.5 h-3.5 text-slate-400" />
              Referenced Table Dependencies
            </label>
            <div className="flex flex-wrap gap-2">
              {(!query.tables || query.tables.length === 0) ? (
                <span className="text-xs text-slate-500 italic">No table dependencies (standalone expression)</span>
              ) : (
                query.tables.map((tbl) => (
                  <span
                    key={tbl}
                    className="inline-flex items-center gap-1 text-xs bg-slate-950 text-indigo-300 border border-slate-800 px-2.5 py-1 rounded-md font-mono"
                  >
                    <CheckCircle className="w-3 h-3 text-emerald-400" />
                    {tbl}
                  </span>
                ))
              )}
            </div>
          </div>
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
