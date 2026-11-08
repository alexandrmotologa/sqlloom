package parser

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
)

// PatternRule defines a regex-based routing directive for queries.
type PatternRule struct {
	ID      string         `json:"id"`
	Pattern string         `json:"pattern"`
	Action  string         `json:"action"` // "BYPASS" or "CACHE"
	TTLSec  int            `json:"ttl_sec,omitempty"`
	regex   *regexp.Regexp `json:"-"`
}

// RulesSummary represents serializable snapshot of active cache rules.
type RulesSummary struct {
	ExcludedTables []string            `json:"excluded_tables"`
	TableTTLs      map[string]int      `json:"table_ttls"`
	Patterns       []PatternRule       `json:"patterns"`
}

// RulesEngine manages user-defined dynamic cache policies.
type RulesEngine struct {
	mu             sync.RWMutex
	excludedTables map[string]bool
	tableTTLs      map[string]int
	patterns       []PatternRule
}

// GlobalRules is the singleton declarative rules engine.
var GlobalRules = NewRulesEngine()

// NewRulesEngine initializes an empty rule set.
func NewRulesEngine() *RulesEngine {
	return &RulesEngine{
		excludedTables: make(map[string]bool),
		tableTTLs:      make(map[string]int),
		patterns:       make([]PatternRule, 0),
	}
}

// ExcludeTable adds a table to the cache bypass list.
func (r *RulesEngine) ExcludeTable(table string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.excludedTables[strings.ToLower(strings.TrimSpace(table))] = true
}

// IncludeTable removes a table from the bypass list.
func (r *RulesEngine) IncludeTable(table string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.excludedTables, strings.ToLower(strings.TrimSpace(table)))
}

// SetTableTTL sets a custom TTL in seconds for a specific table.
func (r *RulesEngine) SetTableTTL(table string, ttlSec int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	tbl := strings.ToLower(strings.TrimSpace(table))
	if ttlSec <= 0 {
		delete(r.tableTTLs, tbl)
	} else {
		r.tableTTLs[tbl] = ttlSec
	}
}

// AddPatternRule adds a regex pattern rule.
func (r *RulesEngine) AddPatternRule(id, pattern, action string, ttlSec int) error {
	re, err := regexp.Compile("(?i)" + pattern)
	if err != nil {
		return fmt.Errorf("invalid pattern regex: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Replace if id exists
	for i, p := range r.patterns {
		if p.ID == id {
			r.patterns[i] = PatternRule{
				ID:      id,
				Pattern: pattern,
				Action:  strings.ToUpper(action),
				TTLSec:  ttlSec,
				regex:   re,
			}
			return nil
		}
	}

	r.patterns = append(r.patterns, PatternRule{
		ID:      id,
		Pattern: pattern,
		Action:  strings.ToUpper(action),
		TTLSec:  ttlSec,
		regex:   re,
	})
	return nil
}

// RemovePatternRule deletes a regex pattern rule by ID.
func (r *RulesEngine) RemovePatternRule(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	for i, p := range r.patterns {
		if p.ID == id {
			r.patterns = append(r.patterns[:i], r.patterns[i+1:]...)
			return true
		}
	}
	return false
}

// EvaluateRules checks declarative policies against a query and its tables.
func (r *RulesEngine) EvaluateRules(sql, normSQL string, tables []string) (forcedBypass bool, reason string, customTTL int) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// 1. Check table exclusions
	for _, rawTbl := range tables {
		tbl := strings.ToLower(rawTbl)
		if r.excludedTables[tbl] {
			return true, fmt.Sprintf("table_excluded:%s", tbl), 0
		}
		// If table has specific schema e.g. public.users, check bare name as well
		if strings.Contains(tbl, ".") {
			parts := strings.Split(tbl, ".")
			bare := parts[len(parts)-1]
			if r.excludedTables[bare] {
				return true, fmt.Sprintf("table_excluded:%s", bare), 0
			}
		}
	}

	// 2. Check pattern rules
	for _, p := range r.patterns {
		if p.regex.MatchString(sql) || p.regex.MatchString(normSQL) {
			if p.Action == "BYPASS" {
				return true, fmt.Sprintf("pattern_rule:%s", p.ID), 0
			}
			if p.Action == "CACHE" && p.TTLSec > 0 {
				customTTL = p.TTLSec
			}
		}
	}

	// 3. Check table-specific TTL overrides (minimum custom TTL applies if multiple tables)
	for _, rawTbl := range tables {
		tbl := strings.ToLower(rawTbl)
		if ttl, ok := r.tableTTLs[tbl]; ok && ttl > 0 {
			if customTTL == 0 || ttl < customTTL {
				customTTL = ttl
			}
		}
	}

	return false, "", customTTL
}

// GetSummary returns an immutable snapshot of current declarative rules.
func (r *RulesEngine) GetSummary() RulesSummary {
	r.mu.RLock()
	defer r.mu.RUnlock()

	excluded := make([]string, 0, len(r.excludedTables))
	for t := range r.excludedTables {
		excluded = append(excluded, t)
	}

	ttls := make(map[string]int, len(r.tableTTLs))
	for t, ttl := range r.tableTTLs {
		ttls[t] = ttl
	}

	patterns := make([]PatternRule, len(r.patterns))
	copy(patterns, r.patterns)

	return RulesSummary{
		ExcludedTables: excluded,
		TableTTLs:      ttls,
		Patterns:       patterns,
	}
}
