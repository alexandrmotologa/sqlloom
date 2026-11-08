package parser

import (
	"testing"

	"github.com/alexandrmotologa/sqlloom/internal/wire"
)

func TestNormalizeQuery(t *testing.T) {
	sql := `
		-- Get active users
		SELECT id, name, email 
		FROM users 
		WHERE org_id = 42 AND active = 'true' /* filter */
	`
	norm, directives := NormalizeQuery(sql)
	expected := "select id, name, email from users where org_id = ? and active = ?"
	if norm != expected {
		t.Fatalf("expected '%s', got '%s'", expected, norm)
	}
	if len(directives) != 0 {
		t.Fatalf("expected 0 directives, got %d", len(directives))
	}
}

func TestNormalizeQueryDirectives(t *testing.T) {
	sql := "SELECT * FROM products /* sqlloom:nocache */ WHERE id = 10"
	norm, directives := NormalizeQuery(sql)
	if len(directives) != 1 || directives[0] != "nocache" {
		t.Fatalf("expected directive 'nocache', got %v", directives)
	}
	expected := "select * from products where id = ?"
	if norm != expected {
		t.Fatalf("expected '%s', got '%s'", expected, norm)
	}
}

func TestExtractTableDependencies(t *testing.T) {
	tests := []struct {
		name     string
		sql      string
		expected []string
		kind     StatementKind
	}{
		{
			name:     "Simple SELECT",
			sql:      "SELECT * FROM users WHERE id = 1",
			expected: []string{"users", "public.users"},
			kind:     StmtSelect,
		},
		{
			name:     "SELECT with JOINs and aliases",
			sql:      "SELECT u.name, o.total FROM users u LEFT JOIN orders o ON u.id = o.user_id WHERE u.active = true",
			expected: []string{"users", "public.users", "orders", "public.orders"},
			kind:     StmtSelect,
		},
		{
			name:     "INSERT statement",
			sql:      "INSERT INTO customers (id, name) VALUES (1, 'Acme')",
			expected: []string{"customers", "public.customers"},
			kind:     StmtInsert,
		},
		{
			name:     "UPDATE statement",
			sql:      "UPDATE products SET price = 99.9 WHERE sku = 'ABC'",
			expected: []string{"products", "public.products"},
			kind:     StmtUpdate,
		},
		{
			name:     "DELETE statement",
			sql:      "DELETE FROM sessions WHERE expired_at < NOW()",
			expected: []string{"sessions", "public.sessions"},
			kind:     StmtDelete,
		},
		{
			name:     "TRUNCATE statement",
			sql:      "TRUNCATE TABLE audit_logs, temp_events CASCADE",
			expected: []string{"audit_logs", "public.audit_logs", "temp_events", "public.temp_events"},
			kind:     StmtTruncate,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref := ExtractTableDependencies(tt.sql)
			if ref.Kind != tt.kind {
				t.Errorf("expected kind %s, got %s", tt.kind, ref.Kind)
			}
			for _, exp := range tt.expected {
				found := false
				for _, got := range ref.Tables {
					if got == exp {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected table %s in %v", exp, ref.Tables)
				}
			}
		})
	}
}

func TestEvaluateCacheability(t *testing.T) {
	// Cacheable query
	dec1 := EvaluateCacheability("SELECT id, name FROM users WHERE id = 42", wire.TxStatusIdle)
	if !dec1.Cacheable {
		t.Fatalf("expected query to be cacheable, got reason: %s", dec1.Reason)
	}

	// Transaction active
	dec2 := EvaluateCacheability("SELECT id, name FROM users", wire.TxStatusInTx)
	if dec2.Cacheable || dec2.Reason != "active_transaction" {
		t.Fatalf("expected active_transaction reason, got %v", dec2)
	}

	// Volatile function
	dec3 := EvaluateCacheability("SELECT id, now() FROM users", wire.TxStatusIdle)
	if dec3.Cacheable || dec3.Reason != "volatile_function" {
		t.Fatalf("expected volatile_function, got %v", dec3)
	}

	// Explicit nocache directive
	dec4 := EvaluateCacheability("SELECT * FROM items /* sqlloom:nocache */", wire.TxStatusIdle)
	if dec4.Cacheable || dec4.Reason != "directive_nocache" {
		t.Fatalf("expected directive_nocache, got %v", dec4)
	}

	// Row locking FOR UPDATE
	dec5 := EvaluateCacheability("SELECT * FROM inventory WHERE item_id = 5 FOR UPDATE", wire.TxStatusIdle)
	if dec5.Cacheable || dec5.Reason != "locking_clause" {
		t.Fatalf("expected locking_clause, got %v", dec5)
	}
}
