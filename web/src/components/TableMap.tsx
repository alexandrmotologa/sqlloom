import React, { useState } from 'react';
import { Table, Trash2, CheckCircle2, RefreshCw } from 'lucide-react';
import { TableStat } from '../types';

interface TableMapProps {
  tables: TableStat[];
  onPurgeTable: (tableName: string) => Promise<void>;
}

export const TableMap: React.FC<TableMapProps> = ({ tables, onPurgeTable }) => {
  const [purgingTable, setPurgingTable] = useState<string | null>(null);
  const [purgedSuccess, setPurgedSuccess] = useState<string | null>(null);

  const handlePurge = async (name: string) => {
    setPurgingTable(name);
    try {
      await onPurgeTable(name);
      setPurgedSuccess(name);
      setTimeout(() => setPurgedSuccess(null), 2500);
    } finally {
      setPurgingTable(null);
    }
  };

  return (
    <div className="bg-slate-900/90 border border-slate-800 rounded-xl overflow-hidden shadow-sm flex flex-col h-[560px]">
      <div className="p-4 border-b border-slate-800 flex items-center justify-between">
        <div className="flex items-center space-x-2">
          <Table className="w-4 h-4 text-indigo-400" />
          <h2 className="text-sm font-semibold text-white tracking-wide uppercase">
            Table Dependency & Invalidation Map
          </h2>
        </div>
        <span className="text-xs text-slate-400 font-mono">
          {tables.length} tables tracked
        </span>
      </div>

      <div className="flex-1 overflow-y-auto p-4 space-y-3">
        {tables.length === 0 ? (
          <div className="text-center py-16 text-slate-500 text-xs">
            No cached tables yet. As applications issue SELECT queries, table dependency trees will populate here automatically.
          </div>
        ) : (
          tables.map((t) => (
            <div
              key={t.name}
              className="bg-slate-950/80 border border-slate-800/80 rounded-lg p-3 flex items-center justify-between hover:border-slate-700 transition-colors"
            >
              <div className="flex items-center space-x-3">
                <div className="w-8 h-8 rounded-lg bg-indigo-500/10 border border-indigo-500/20 flex items-center justify-center text-indigo-400">
                  <Table className="w-4 h-4" />
                </div>
                <div>
                  <h3 className="text-xs font-mono font-semibold text-white">
                    {t.name}
                  </h3>
                  <p className="text-[11px] text-slate-400">
                    <strong className="text-slate-200">{t.cached_queries}</strong> active cached query variants
                  </p>
                </div>
              </div>

              <div className="flex items-center space-x-3">
                {purgedSuccess === t.name ? (
                  <span className="text-xs text-emerald-400 flex items-center space-x-1 font-mono">
                    <CheckCircle2 className="w-3.5 h-3.5" />
                    <span>Purged</span>
                  </span>
                ) : (
                  <button
                    onClick={() => handlePurge(t.name)}
                    disabled={purgingTable === t.name}
                    className="flex items-center space-x-1.5 px-2.5 py-1 text-xs font-medium text-slate-300 hover:text-rose-400 bg-slate-900 hover:bg-rose-950/30 border border-slate-800 hover:border-rose-900/50 rounded-md transition-colors"
                    title="1-Click evict all cached queries for this table"
                  >
                    {purgingTable === t.name ? (
                      <RefreshCw className="w-3.5 h-3.5 animate-spin" />
                    ) : (
                      <Trash2 className="w-3.5 h-3.5" />
                    )}
                    <span>Purge Table</span>
                  </button>
                )}
              </div>
            </div>
          ))
        )}
      </div>

      <div className="p-3 bg-slate-950 border-t border-slate-800 text-[11px] text-slate-400 flex items-center justify-between">
        <span>Automatic invalidation active on INSERT, UPDATE, DELETE</span>
        <span className="text-indigo-400 font-mono">Instant purge on write</span>
      </div>
    </div>
  );
};
