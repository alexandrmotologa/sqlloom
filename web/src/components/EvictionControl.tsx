import React, { useState } from 'react';
import { Trash2, AlertTriangle, CheckCircle, RefreshCw, Cpu } from 'lucide-react';

interface EvictionControlProps {
  onPurgeAll: () => Promise<number>;
}

export const EvictionControl: React.FC<EvictionControlProps> = ({ onPurgeAll }) => {
  const [confirming, setConfirming] = useState(false);
  const [purging, setPurging] = useState(false);
  const [purgedCount, setPurgedCount] = useState<number | null>(null);

  const handlePurge = async () => {
    setPurging(true);
    try {
      const count = await onPurgeAll();
      setPurgedCount(count);
      setConfirming(false);
      setTimeout(() => setPurgedCount(null), 3000);
    } finally {
      setPurging(false);
    }
  };

  return (
    <div className="bg-slate-900/90 border border-slate-800 rounded-xl p-4 shadow-sm flex flex-col sm:flex-row sm:items-center justify-between gap-4">
      <div className="flex items-center space-x-3">
        <div className="w-9 h-9 rounded-lg bg-indigo-500/10 border border-indigo-500/20 flex items-center justify-center text-indigo-400">
          <Cpu className="w-5 h-5" />
        </div>
        <div>
          <h3 className="text-xs font-semibold text-white uppercase tracking-wider">
            In-Memory Acceleration Engine
          </h3>
          <p className="text-xs text-slate-400">
            Automated write-through invalidation active. Lock-free 16-stripe LRU cache bounded to &lt; 35MB RAM.
          </p>
        </div>
      </div>

      <div className="flex items-center space-x-3">
        {purgedCount !== null ? (
          <span className="text-xs text-emerald-400 flex items-center gap-1 font-mono">
            <CheckCircle className="w-4 h-4" />
            <span>Flushed {purgedCount} entries</span>
          </span>
        ) : confirming ? (
          <div className="flex items-center space-x-2 animate-in fade-in">
            <span className="text-xs text-amber-300 flex items-center gap-1">
              <AlertTriangle className="w-3.5 h-3.5" />
              Flush all cache?
            </span>
            <button
              onClick={handlePurge}
              disabled={purging}
              className="px-2.5 py-1 bg-rose-600 hover:bg-rose-500 text-white rounded text-xs font-semibold flex items-center gap-1 transition-colors"
            >
              {purging && <RefreshCw className="w-3 h-3 animate-spin" />}
              Confirm
            </button>
            <button
              onClick={() => setConfirming(false)}
              className="px-2.5 py-1 bg-slate-800 hover:bg-slate-700 text-slate-300 rounded text-xs transition-colors"
            >
              Cancel
            </button>
          </div>
        ) : (
          <button
            onClick={() => setConfirming(true)}
            className="flex items-center space-x-1.5 px-3 py-1.5 text-xs font-medium text-slate-300 hover:text-white bg-slate-800 hover:bg-slate-700 border border-slate-700 rounded-lg transition-colors"
          >
            <Trash2 className="w-3.5 h-3.5 text-rose-400" />
            <span>Flush Entire Cache</span>
          </button>
        )}
      </div>
    </div>
  );
};
