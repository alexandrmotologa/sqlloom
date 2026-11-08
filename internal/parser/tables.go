package parser

import (
	"strings"
	"unicode"
)

// StatementKind represents the SQL command category.
type StatementKind string

const (
	StmtSelect   StatementKind = "SELECT"
	StmtInsert   StatementKind = "INSERT"
	StmtUpdate   StatementKind = "UPDATE"
	StmtDelete   StatementKind = "DELETE"
	StmtTruncate StatementKind = "TRUNCATE"
	StmtBegin    StatementKind = "BEGIN"
	StmtCommit   StatementKind = "COMMIT"
	StmtRollback StatementKind = "ROLLBACK"
	StmtDDL      StatementKind = "DDL"
	StmtOther    StatementKind = "OTHER"
)

// TableReference holds extracted table names.
type TableReference struct {
	Tables []string
	Kind   StatementKind
}

// ExtractTableDependencies analyzes a SQL query and returns statement kind and referenced tables.
func ExtractTableDependencies(sql string) TableReference {
	tokens := tokenize(sql)
	if len(tokens) == 0 {
		return TableReference{Kind: StmtOther}
	}

	upperTokens := make([]string, len(tokens))
	for i, t := range tokens {
		upperTokens[i] = strings.ToUpper(t)
	}

	first := upperTokens[0]

	// Handle Transaction statements
	switch first {
	case "BEGIN", "START":
		return TableReference{Kind: StmtBegin}
	case "COMMIT", "END":
		return TableReference{Kind: StmtCommit}
	case "ROLLBACK", "ABORT":
		return TableReference{Kind: StmtRollback}
	case "ALTER", "DROP", "CREATE":
		tables := extractDDLTables(tokens, upperTokens)
		return TableReference{Tables: tables, Kind: StmtDDL}
	case "INSERT":
		tables := extractInsertTables(tokens, upperTokens)
		return TableReference{Tables: tables, Kind: StmtInsert}
	case "UPDATE":
		tables := extractUpdateTables(tokens, upperTokens)
		return TableReference{Tables: tables, Kind: StmtUpdate}
	case "DELETE":
		tables := extractDeleteTables(tokens, upperTokens)
		return TableReference{Tables: tables, Kind: StmtDelete}
	case "TRUNCATE":
		tables := extractTruncateTables(tokens, upperTokens)
		return TableReference{Tables: tables, Kind: StmtTruncate}
	case "SELECT", "WITH":
		tables := extractSelectTables(tokens, upperTokens)
		return TableReference{Tables: tables, Kind: StmtSelect}
	default:
		return TableReference{Kind: StmtOther}
	}
}

func extractInsertTables(tokens, upper []string) []string {
	for i := 0; i < len(upper); i++ {
		if upper[i] == "INTO" && i+1 < len(tokens) {
			tbl := cleanTableName(tokens[i+1])
			if tbl != "" {
				return expandSchemaTable(tbl)
			}
		}
	}
	return nil
}

func extractUpdateTables(tokens, upper []string) []string {
	if len(tokens) > 1 {
		tbl := cleanTableName(tokens[1])
		if tbl != "" {
			return expandSchemaTable(tbl)
		}
	}
	return nil
}

func extractDeleteTables(tokens, upper []string) []string {
	for i := 0; i < len(upper); i++ {
		if upper[i] == "FROM" && i+1 < len(tokens) {
			tbl := cleanTableName(tokens[i+1])
			if tbl != "" {
				return expandSchemaTable(tbl)
			}
		}
	}
	return nil
}

func extractTruncateTables(tokens, upper []string) []string {
	var tables []string
	start := 1
	if start < len(upper) && upper[start] == "TABLE" {
		start++
	}
	for i := start; i < len(tokens); i++ {
		tok := cleanTableName(tokens[i])
		if tok == "" || tok == "," || tok == ";" || tok == "cascade" || tok == "restrict" {
			continue
		}
		tables = append(tables, expandSchemaTable(tok)...)
	}
	return deduplicate(tables)
}

func extractDDLTables(tokens, upper []string) []string {
	for i := 0; i < len(upper)-1; i++ {
		if upper[i] == "TABLE" {
			target := i + 1
			if upper[target] == "IF" && target+2 < len(upper) && upper[target+1] == "EXISTS" {
				target += 2
			}
			if target < len(tokens) {
				tbl := cleanTableName(tokens[target])
				if tbl != "" {
					return expandSchemaTable(tbl)
				}
			}
		}
	}
	return nil
}

func extractSelectTables(tokens, upper []string) []string {
	var tables []string
	n := len(tokens)

	for i := 0; i < n; i++ {
		u := upper[i]

		// Table in FROM clause
		if u == "FROM" && i+1 < n {
			i++
			for i < n {
				if tokens[i] == "(" {
					// Skip subquery parenthesized expression
					parenDepth := 1
					i++
					for i < n && parenDepth > 0 {
						if tokens[i] == "(" {
							parenDepth++
						} else if tokens[i] == ")" {
							parenDepth--
						}
						i++
					}
					// Skip subquery alias if present
					if i < n && isIdentifier(tokens[i]) && !isClauseKeyword(upper[i]) {
						i++
					}
				} else if isIdentifier(tokens[i]) {
					tbl := cleanTableName(tokens[i])
					if tbl != "" && !isClauseKeyword(upper[i]) {
						tables = append(tables, expandSchemaTable(tbl)...)
					}
					i++
					// Skip table alias if present
					if i < n && isIdentifier(tokens[i]) && upper[i] != "WHERE" && upper[i] != "JOIN" && upper[i] != "LEFT" && upper[i] != "RIGHT" && upper[i] != "INNER" && upper[i] != "FULL" && upper[i] != "CROSS" && upper[i] != "ON" && upper[i] != "USING" && upper[i] != "GROUP" && upper[i] != "ORDER" && upper[i] != "LIMIT" && upper[i] != "," {
						i++
					}
				}

				if i < n && tokens[i] == "," {
					i++ // Next table in comma-separated list
					continue
				}
				break
			}
			continue
		}

		// Table in JOIN clauses
		if (u == "JOIN" || (i+1 < n && (u == "LEFT" || u == "RIGHT" || u == "INNER" || u == "FULL" || u == "CROSS") && upper[i+1] == "JOIN")) {
			targetIdx := i + 1
			if upper[targetIdx] == "JOIN" {
				targetIdx++
			}
			if targetIdx < n {
				if tokens[targetIdx] == "(" {
					// subquery join, let loop continue
					i = targetIdx
					continue
				}
				tbl := cleanTableName(tokens[targetIdx])
				if tbl != "" && !isClauseKeyword(upper[targetIdx]) {
					tables = append(tables, expandSchemaTable(tbl)...)
				}
				i = targetIdx
			}
		}
	}

	return deduplicate(tables)
}

func tokenize(sql string) []string {
	var tokens []string
	runes := []rune(sql)
	n := len(runes)
	i := 0

	for i < n {
		r := runes[i]

		// Skip whitespace
		if unicode.IsSpace(r) {
			i++
			continue
		}

		// Single-line comment -- ...
		if r == '-' && i+1 < n && runes[i+1] == '-' {
			i += 2
			for i < n && runes[i] != '\n' && runes[i] != '\r' {
				i++
			}
			continue
		}

		// Block comment /* ... */
		if r == '/' && i+1 < n && runes[i+1] == '*' {
			i += 2
			for i+1 < n && !(runes[i] == '*' && runes[i+1] == '/') {
				i++
			}
			i += 2
			continue
		}

		// Quoted identifier "table_name"
		if r == '"' {
			start := i
			i++
			for i < n && runes[i] != '"' {
				i++
			}
			if i < n {
				i++
			}
			tokens = append(tokens, string(runes[start:i]))
			continue
		}

		// String literal 'text'
		if r == '\'' {
			start := i
			i++
			for i < n {
				if runes[i] == '\'' {
					if i+1 < n && runes[i+1] == '\'' {
						i += 2
						continue
					}
					i++
					break
				}
				i++
			}
			tokens = append(tokens, string(runes[start:i]))
			continue
		}

		// Single character punctuation tokens
		if r == '(' || r == ')' || r == ',' || r == ';' {
			tokens = append(tokens, string(r))
			i++
			continue
		}

		// Identifiers and numbers
		start := i
		for i < n && !unicode.IsSpace(runes[i]) && runes[i] != '(' && runes[i] != ')' && runes[i] != ',' && runes[i] != ';' && runes[i] != '\'' && runes[i] != '"' {
			i++
		}
		if i > start {
			tokens = append(tokens, string(runes[start:i]))
		}
	}

	return tokens
}

func cleanTableName(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.Trim(raw, ";,()")
	raw = strings.ReplaceAll(raw, "\"", "")
	return strings.ToLower(raw)
}

func expandSchemaTable(table string) []string {
	if strings.Contains(table, ".") {
		parts := strings.Split(table, ".")
		bare := parts[len(parts)-1]
		return []string{table, bare}
	}
	return []string{table, "public." + table}
}

func isIdentifier(s string) bool {
	if s == "" || s == "(" || s == ")" || s == "," || s == ";" {
		return false
	}
	return true
}

func isClauseKeyword(u string) bool {
	switch u {
	case "WHERE", "GROUP", "ORDER", "HAVING", "LIMIT", "OFFSET", "UNION", "EXCEPT", "INTERSECT", "ON", "USING", "JOIN", "LEFT", "RIGHT", "INNER", "FULL", "CROSS", "NATURAL":
		return true
	default:
		return false
	}
}

func deduplicate(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	result := make([]string, 0, len(items))
	for _, item := range items {
		if item == "" {
			continue
		}
		if _, exists := seen[item]; !exists {
			seen[item] = struct{}{}
			result = append(result, item)
		}
	}
	return result
}
