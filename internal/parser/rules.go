package parser

import (
	"strings"

	"github.com/alexandrmotologa/sqlloom/internal/wire"
)

var volatileFunctions = []string{
	"now()",
	"current_timestamp",
	"current_time",
	"current_date",
	"localtime",
	"localtimestamp",
	"random()",
	"gen_random_uuid()",
	"uuid_generate_v4()",
	"uuid_generate_v1()",
	"clock_timestamp()",
	"statement_timestamp()",
	"timeofday()",
}

var lockingKeywords = []string{
	"for update",
	"for share",
	"for no key update",
	"for key share",
}

var systemCatalogs = []string{
	"pg_catalog",
	"information_schema",
	"pg_stat",
	"pg_class",
	"pg_type",
	"pg_database",
}

// CacheDecision describes whether a statement can be cached.
type CacheDecision struct {
	Cacheable  bool
	Reason     string
	Kind       StatementKind
	Tables     []string
	CustomTTL  int // Optional custom TTL in seconds from directive
}

// EvaluateCacheability checks transaction status, SQL grammar, and directives.
func EvaluateCacheability(sql string, txStatus byte) CacheDecision {
	ref := ExtractTableDependencies(sql)

	// Rule 1: Active transaction blocks bypass caching for ACID consistency
	if txStatus != wire.TxStatusIdle {
		return CacheDecision{
			Cacheable: false,
			Reason:    "active_transaction",
			Kind:      ref.Kind,
			Tables:    ref.Tables,
		}
	}

	normalized, directives := NormalizeQuery(sql)
	lowerNorm := strings.ToLower(normalized)

	// Rule 2: Explicit nocache directive
	for _, d := range directives {
		if strings.HasPrefix(d, "nocache") {
			return CacheDecision{
				Cacheable: false,
				Reason:    "directive_nocache",
				Kind:      StmtSelect,
			}
		}
	}

	if ref.Kind != StmtSelect {
		return CacheDecision{
			Cacheable: false,
			Reason:    "non_select_statement",
			Kind:      ref.Kind,
			Tables:    ref.Tables,
		}
	}

	// Must reference at least one user table
	if len(ref.Tables) == 0 {
		return CacheDecision{
			Cacheable: false,
			Reason:    "no_table_dependencies",
			Kind:      StmtSelect,
		}
	}

	// Rule 3: System catalog exclusion
	for _, cat := range systemCatalogs {
		if strings.Contains(lowerNorm, cat) {
			return CacheDecision{
				Cacheable: false,
				Reason:    "system_catalog",
				Kind:      StmtSelect,
				Tables:    ref.Tables,
			}
		}
	}

	// Rule 4: Volatile functions
	for _, fn := range volatileFunctions {
		if strings.Contains(lowerNorm, fn) {
			return CacheDecision{
				Cacheable: false,
				Reason:    "volatile_function",
				Kind:      StmtSelect,
				Tables:    ref.Tables,
			}
		}
	}

	// Rule 5: Row locking clauses
	for _, lock := range lockingKeywords {
		if strings.Contains(lowerNorm, lock) {
			return CacheDecision{
				Cacheable: false,
				Reason:    "locking_clause",
				Kind:      StmtSelect,
				Tables:    ref.Tables,
			}
		}
	}

	// Rule 6: Modifying CTEs
	if strings.Contains(lowerNorm, "with ") && (strings.Contains(lowerNorm, "delete from") || strings.Contains(lowerNorm, "update ") || strings.Contains(lowerNorm, "insert into")) {
		return CacheDecision{
			Cacheable: false,
			Reason:    "modifying_cte",
			Kind:      StmtSelect,
			Tables:    ref.Tables,
		}
	}

	// Rule 7: Declarative user policies (table exclusions, regex rules, custom TTL)
	if forcedBypass, reason, customTTL := GlobalRules.EvaluateRules(sql, lowerNorm, ref.Tables); forcedBypass {
		return CacheDecision{
			Cacheable: false,
			Reason:    reason,
			Kind:      StmtSelect,
			Tables:    ref.Tables,
		}
	} else if customTTL > 0 {
		return CacheDecision{
			Cacheable: true,
			Reason:    "cacheable",
			Kind:      StmtSelect,
			Tables:    ref.Tables,
			CustomTTL: customTTL,
		}
	}

	return CacheDecision{
		Cacheable: true,
		Reason:    "cacheable",
		Kind:      StmtSelect,
		Tables:    ref.Tables,
	}
}
