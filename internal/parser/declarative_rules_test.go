package parser

import (
	"testing"
)

func TestDeclarativeRulesEngine(t *testing.T) {
	rules := NewRulesEngine()

	// 1. Table exclusion
	rules.ExcludeTable("session_tokens")
	bypass, reason, _ := rules.EvaluateRules("SELECT * FROM session_tokens", "select * from session_tokens", []string{"session_tokens"})
	if !bypass || reason != "table_excluded:session_tokens" {
		t.Fatalf("expected table exclusion for session_tokens, got bypass=%v, reason=%s", bypass, reason)
	}

	// 2. Remove table exclusion
	rules.IncludeTable("session_tokens")
	bypass, _, _ = rules.EvaluateRules("SELECT * FROM session_tokens", "select * from session_tokens", []string{"session_tokens"})
	if bypass {
		t.Fatalf("expected session_tokens to no longer be bypassed")
	}

	// 3. Table custom TTL
	rules.SetTableTTL("products", 120)
	_, _, ttl := rules.EvaluateRules("SELECT * FROM products", "select * from products", []string{"products"})
	if ttl != 120 {
		t.Fatalf("expected custom TTL 120, got %d", ttl)
	}

	// 4. Pattern regex rule
	err := rules.AddPatternRule("rule-1", "SELECT .* FROM audit_log.*", "BYPASS", 0)
	if err != nil {
		t.Fatalf("unexpected error adding pattern rule: %v", err)
	}

	bypass, reason, _ = rules.EvaluateRules("SELECT * FROM audit_logs WHERE id = 1", "select * from audit_logs where id = ?", []string{"audit_logs"})
	if !bypass || reason != "pattern_rule:rule-1" {
		t.Fatalf("expected pattern_rule:rule-1 bypass, got bypass=%v, reason=%s", bypass, reason)
	}
}
