package parser

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"unicode"
)

// NormalizeQuery strips comments, collapses whitespace, and replaces literals with placeholders '?'.
func NormalizeQuery(sql string) (normalized string, directives []string) {
	// First pass: extract any /* sqlloom:... */ directives
	directives = extractDirectives(sql)

	var sb strings.Builder
	sb.Grow(len(sql))

	runes := []rune(sql)
	n := len(runes)
	i := 0
	inSpace := false

	for i < n {
		r := runes[i]

		// Line comment -- ...
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

		// String literal '...'
		if r == '\'' {
			sb.WriteRune('?')
			i++
			for i < n {
				if runes[i] == '\'' {
					if i+1 < n && runes[i+1] == '\'' {
						i += 2 // Escaped quote ''
						continue
					}
					i++
					break
				}
				i++
			}
			inSpace = false
			continue
		}

		// Numeric literal (standalone number)
		if unicode.IsDigit(r) && (i == 0 || isBoundary(runes[i-1])) {
			sb.WriteRune('?')
			for i < n && (unicode.IsDigit(runes[i]) || runes[i] == '.') {
				i++
			}
			inSpace = false
			continue
		}

		// Whitespace collapsing
		if unicode.IsSpace(r) {
			if !inSpace && sb.Len() > 0 {
				sb.WriteRune(' ')
				inSpace = true
			}
			i++
			continue
		}

		sb.WriteRune(unicode.ToLower(r))
		inSpace = false
		i++
	}

	normalized = strings.TrimSpace(sb.String())
	return normalized, directives
}

// ComputeCacheKey calculates a deterministic SHA-256 hash for database + normalized SQL + parameters.
func ComputeCacheKey(database, normalizedSQL string, params []string) string {
	h := sha256.New()
	h.Write([]byte(database))
	h.Write([]byte{0})
	h.Write([]byte(normalizedSQL))
	h.Write([]byte{0})

	for _, p := range params {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}

	return hex.EncodeToString(h.Sum(nil))
}

func extractDirectives(sql string) []string {
	var directives []string
	lower := strings.ToLower(sql)
	idx := 0
	for {
		start := strings.Index(lower[idx:], "/* sqlloom:")
		if start == -1 {
			break
		}
		start += idx + len("/* sqlloom:")
		end := strings.Index(lower[start:], "*/")
		if end == -1 {
			break
		}
		directive := strings.TrimSpace(lower[start : start+end])
		if directive != "" {
			directives = append(directives, directive)
		}
		idx = start + end + 2
	}
	return directives
}

func isBoundary(r rune) bool {
	return unicode.IsSpace(r) || r == '(' || r == ',' || r == '=' || r == '<' || r == '>' || r == '+' || r == '-' || r == '*' || r == '/'
}
