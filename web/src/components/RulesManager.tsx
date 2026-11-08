import React, { useState } from 'react';
import { ShieldBan, Clock, Code2, Plus, Trash2, CheckCircle2 } from 'lucide-react';
import { RulesSummary } from '../types';

interface RulesManagerProps {
  rules: RulesSummary | null;
  onRefresh: () => void;
}

export const RulesManager: React.FC<RulesManagerProps> = ({ rules, onRefresh }) => {
  const [excludeInput, setExcludeInput] = useState('');
  const [ttlTableInput, setTtlTableInput] = useState('');
  const [ttlSecInput, setTtlSecInput] = useState('300');
  const [patIdInput, setPatIdInput] = useState('');
  const [patRegexInput, setPatRegexInput] = useState('');
  const [patActionInput, setPatActionInput] = useState<'BYPASS' | 'CACHE'>('BYPASS');
  const [patTtlInput, setPatTtlInput] = useState('60');

  const [feedback, setFeedback] = useState<string | null>(null);

  const showFeedback = (msg: string) => {
    setFeedback(msg);
    setTimeout(() => setFeedback(null), 2500);
  };

  const handleAddExclude = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!excludeInput.trim()) return;

    await fetch('/api/rules/exclude', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ table: excludeInput.trim(), exclude: true }),
    });
    setExcludeInput('');
    onRefresh();
    showFeedback(`Table '${excludeInput.trim()}' excluded from cache.`);
  };

  const handleRemoveExclude = async (table: string) => {
    await fetch('/api/rules/exclude', {
      method: 'DELETE',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ table, exclude: false }),
    });
    onRefresh();
    showFeedback(`Table '${table}' restored to cache.`);
  };

  const handleSetTTL = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!ttlTableInput.trim() || !ttlSecInput) return;

    await fetch('/api/rules/ttl', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ table: ttlTableInput.trim(), ttl_sec: parseInt(ttlSecInput, 10) }),
    });
    setTtlTableInput('');
    onRefresh();
    showFeedback(`Custom TTL set for '${ttlTableInput.trim()}'.`);
  };

  const handleRemoveTTL = async (table: string) => {
    await fetch('/api/rules/ttl', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ table, ttl_sec: 0 }),
    });
    onRefresh();
    showFeedback(`Custom TTL removed for '${table}'.`);
  };

  const handleAddPattern = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!patRegexInput.trim()) return;

    await fetch('/api/rules/pattern', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        id: patIdInput.trim() || undefined,
        pattern: patRegexInput.trim(),
        action: patActionInput,
        ttl_sec: patActionInput === 'CACHE' ? parseInt(patTtlInput, 10) : 0,
      }),
    });
    setPatIdInput('');
    setPatRegexInput('');
    onRefresh();
    showFeedback(`Regex pattern rule registered.`);
  };

  const handleDeletePattern = async (id: string) => {
    await fetch('/api/rules/pattern', {
      method: 'DELETE',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ id }),
    });
    onRefresh();
    showFeedback(`Rule '${id}' deleted.`);
  };

  return (
    <div className="space-y-6">
      {feedback && (
        <div className="bg-emerald-500/10 border border-emerald-500/30 text-emerald-400 text-xs px-4 py-2.5 rounded-lg flex items-center space-x-2 animate-in fade-in">
          <CheckCircle2 className="w-4 h-4 shrink-0" />
          <span>{feedback}</span>
        </div>
      )}

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Section 1: Excluded Tables */}
        <div className="bg-slate-900/90 border border-slate-800 rounded-xl p-5 shadow-sm space-y-4">
          <div className="flex items-center space-x-2.5 pb-3 border-b border-slate-800">
            <ShieldBan className="w-4 h-4 text-rose-400" />
            <div>
              <h3 className="text-xs font-semibold text-white uppercase tracking-wider">
                Table Exclusions (Bypass Cache)
              </h3>
              <p className="text-[11px] text-slate-400">
                Queries touching these tables will never be cached.
              </p>
            </div>
          </div>

          <form onSubmit={handleAddExclude} className="flex gap-2">
            <input
              type="text"
              placeholder="e.g. session_tokens, heartbeats"
              value={excludeInput}
              onChange={(e) => setExcludeInput(e.target.value)}
              className="flex-1 bg-slate-950 border border-slate-800 rounded-lg px-3 py-1.5 text-xs text-white placeholder-slate-500 focus:outline-none focus:border-indigo-500 font-mono"
            />
            <button
              type="submit"
              className="px-3 py-1.5 bg-rose-600 hover:bg-rose-500 text-white rounded-lg text-xs font-medium flex items-center space-x-1 transition-colors"
            >
              <Plus className="w-3.5 h-3.5" />
              <span>Exclude</span>
            </button>
          </form>

          <div className="divide-y divide-slate-800/60 max-h-48 overflow-y-auto">
            {(!rules?.excluded_tables || rules.excluded_tables.length === 0) ? (
              <p className="text-xs text-slate-500 py-3 italic">
                No tables excluded. All non-transactional SELECT queries are eligible for caching.
              </p>
            ) : (
              rules.excluded_tables.map((t) => (
                <div key={t} className="py-2 flex items-center justify-between text-xs font-mono">
                  <span className="text-rose-300 bg-rose-950/40 px-2 py-0.5 rounded border border-rose-900/50">
                    {t}
                  </span>
                  <button
                    onClick={() => handleRemoveExclude(t)}
                    className="text-slate-400 hover:text-rose-400 p-1 rounded transition-colors"
                    title="Remove exclusion"
                  >
                    <Trash2 className="w-3.5 h-3.5" />
                  </button>
                </div>
              ))
            )}
          </div>
        </div>

        {/* Section 2: Table Custom TTL */}
        <div className="bg-slate-900/90 border border-slate-800 rounded-xl p-5 shadow-sm space-y-4">
          <div className="flex items-center space-x-2.5 pb-3 border-b border-slate-800">
            <Clock className="w-4 h-4 text-amber-400" />
            <div>
              <h3 className="text-xs font-semibold text-white uppercase tracking-wider">
                Table Custom TTL Overrides
              </h3>
              <p className="text-[11px] text-slate-400">
                Override the default TTL for specific tables.
              </p>
            </div>
          </div>

          <form onSubmit={handleSetTTL} className="flex gap-2">
            <input
              type="text"
              placeholder="Table name (e.g. products)"
              value={ttlTableInput}
              onChange={(e) => setTtlTableInput(e.target.value)}
              className="flex-1 bg-slate-950 border border-slate-800 rounded-lg px-3 py-1.5 text-xs text-white placeholder-slate-500 focus:outline-none focus:border-indigo-500 font-mono"
            />
            <input
              type="number"
              placeholder="TTL (sec)"
              value={ttlSecInput}
              onChange={(e) => setTtlSecInput(e.target.value)}
              className="w-24 bg-slate-950 border border-slate-800 rounded-lg px-3 py-1.5 text-xs text-white placeholder-slate-500 focus:outline-none focus:border-indigo-500 font-mono"
            />
            <button
              type="submit"
              className="px-3 py-1.5 bg-amber-600 hover:bg-amber-500 text-white rounded-lg text-xs font-medium flex items-center space-x-1 transition-colors"
            >
              <Plus className="w-3.5 h-3.5" />
              <span>Set</span>
            </button>
          </form>

          <div className="divide-y divide-slate-800/60 max-h-48 overflow-y-auto">
            {(!rules?.table_ttls || Object.keys(rules.table_ttls).length === 0) ? (
              <p className="text-xs text-slate-500 py-3 italic">
                No custom table TTLs configured. All tables inherit proxy default TTL (3600s).
              </p>
            ) : (
              Object.entries(rules.table_ttls).map(([t, ttl]) => (
                <div key={t} className="py-2 flex items-center justify-between text-xs font-mono">
                  <div className="flex items-center space-x-2">
                    <span className="text-white font-medium">{t}</span>
                    <span className="text-amber-400 bg-amber-950/40 px-2 py-0.5 rounded border border-amber-900/50">
                      {ttl}s
                    </span>
                  </div>
                  <button
                    onClick={() => handleRemoveTTL(t)}
                    className="text-slate-400 hover:text-rose-400 p-1 rounded transition-colors"
                    title="Remove custom TTL"
                  >
                    <Trash2 className="w-3.5 h-3.5" />
                  </button>
                </div>
              ))
            )}
          </div>
        </div>
      </div>

      {/* Section 3: Pattern Regex Rules */}
      <div className="bg-slate-900/90 border border-slate-800 rounded-xl p-5 shadow-sm space-y-4">
        <div className="flex items-center space-x-2.5 pb-3 border-b border-slate-800">
          <Code2 className="w-4 h-4 text-indigo-400" />
          <div>
            <h3 className="text-xs font-semibold text-white uppercase tracking-wider">
              SQL Regex Pattern Routing Rules
            </h3>
            <p className="text-[11px] text-slate-400">
              Match SQL text with regular expressions to force BYPASS or apply custom caching rules.
            </p>
          </div>
        </div>

        <form onSubmit={handleAddPattern} className="flex flex-wrap gap-2">
          <input
            type="text"
            placeholder="Rule Label (optional)"
            value={patIdInput}
            onChange={(e) => setPatIdInput(e.target.value)}
            className="w-40 bg-slate-950 border border-slate-800 rounded-lg px-3 py-1.5 text-xs text-white placeholder-slate-500 focus:outline-none focus:border-indigo-500 font-mono"
          />
          <input
            type="text"
            placeholder="Regex pattern e.g. SELECT .* FROM audit_log.*"
            value={patRegexInput}
            onChange={(e) => setPatRegexInput(e.target.value)}
            className="flex-1 min-w-[240px] bg-slate-950 border border-slate-800 rounded-lg px-3 py-1.5 text-xs text-white placeholder-slate-500 focus:outline-none focus:border-indigo-500 font-mono"
          />
          <select
            value={patActionInput}
            onChange={(e) => setPatActionInput(e.target.value as 'BYPASS' | 'CACHE')}
            className="bg-slate-950 border border-slate-800 rounded-lg px-3 py-1.5 text-xs text-white focus:outline-none focus:border-indigo-500"
          >
            <option value="BYPASS">BYPASS</option>
            <option value="CACHE">CACHE</option>
          </select>
          {patActionInput === 'CACHE' && (
            <input
              type="number"
              placeholder="TTL (sec)"
              value={patTtlInput}
              onChange={(e) => setPatTtlInput(e.target.value)}
              className="w-24 bg-slate-950 border border-slate-800 rounded-lg px-3 py-1.5 text-xs text-white placeholder-slate-500 focus:outline-none focus:border-indigo-500 font-mono"
            />
          )}
          <button
            type="submit"
            className="px-4 py-1.5 bg-indigo-600 hover:bg-indigo-500 text-white rounded-lg text-xs font-medium flex items-center space-x-1 transition-colors"
          >
            <Plus className="w-3.5 h-3.5" />
            <span>Add Rule</span>
          </button>
        </form>

        <div className="divide-y divide-slate-800/60 max-h-56 overflow-y-auto">
          {(!rules?.patterns || rules.patterns.length === 0) ? (
            <p className="text-xs text-slate-500 py-3 italic">
              No custom regex pattern rules defined.
            </p>
          ) : (
            rules.patterns.map((p) => (
              <div key={p.id} className="py-2.5 flex items-center justify-between text-xs font-mono">
                <div className="flex items-center space-x-3">
                  <span
                    className={`px-2 py-0.5 rounded text-[10px] font-bold ${
                      p.action === 'BYPASS'
                        ? 'bg-amber-500/10 text-amber-400 border border-amber-500/20'
                        : 'bg-emerald-500/10 text-emerald-400 border border-emerald-500/20'
                    }`}
                  >
                    {p.action}
                  </span>
                  <span className="text-white font-mono">{p.pattern}</span>
                  {p.ttl_sec ? (
                    <span className="text-[11px] text-slate-400">({p.ttl_sec}s TTL)</span>
                  ) : null}
                  <span className="text-[10px] text-slate-500">[{p.id}]</span>
                </div>
                <button
                  onClick={() => handleDeletePattern(p.id)}
                  className="text-slate-400 hover:text-rose-400 p-1 rounded transition-colors"
                  title="Delete pattern rule"
                >
                  <Trash2 className="w-3.5 h-3.5" />
                </button>
              </div>
            ))
          )}
        </div>
      </div>
    </div>
  );
};
