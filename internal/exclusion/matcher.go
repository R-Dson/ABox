package exclusion

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// Matcher checks paths against a set of exclusion patterns.
type Matcher struct {
	patterns []string
}

// NewMatcher creates a Matcher with the given patterns.
// User-facing patterns are normalized for doublestar compatibility:
//   - "dir/" becomes "**/dir/**" (match directory contents anywhere)
//   - "*.ext" becomes "**/*.ext" (match at any depth)
func NewMatcher(patterns []string) *Matcher {
	return &Matcher{patterns: normalizePatterns(patterns)}
}

// Match returns true if the path matches any exclusion pattern.
func (m *Matcher) Match(path string) bool {
	for _, p := range m.patterns {
		if ok, _ := doublestar.Match(p, path); ok {
			return true
		}
	}
	return false
}

// HasPatterns returns true if the matcher has any patterns.
func (m *Matcher) HasPatterns() bool {
	return len(m.patterns) > 0
}

// BuildMatcher composes patterns from hardcoded security patterns
// and the local .abxignore file in the workspace.
func BuildMatcher(_ context.Context, workdir string) (*Matcher, error) {
	patterns := HardcodedPatterns()

	if local, err := loadLocalIgnore(workdir); err == nil {
		patterns = mergePatterns(patterns, normalizePatterns(local))
	}

	return &Matcher{patterns: patterns}, nil
}

func loadLocalIgnore(workdir string) ([]string, error) {
	path := filepath.Join(workdir, ".abxignore")
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening ignore file: %w", err)
	}
	defer f.Close()

	var patterns []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading ignore file: %w", err)
	}
	return patterns, nil
}

func mergePatterns(base, additional []string) []string {
	seen := make(map[string]bool)
	for _, p := range base {
		seen[p] = true
	}
	for _, p := range additional {
		if !seen[p] {
			base = append(base, p)
			seen[p] = true
		}
	}
	return base
}

// normalizePatterns converts user-facing .gitignore-style patterns
// to doublestar-compatible glob patterns.
func normalizePatterns(patterns []string) []string {
	normalized := make([]string, len(patterns))
	for i, p := range patterns {
		normalized[i] = normalizePattern(p)
	}
	return normalized
}

func normalizePattern(p string) string {
	// Already a doublestar pattern
	if containsGlobstar(p) {
		return p
	}

	// Trailing slash = directory match at any depth: "build/" → "**/build/**"
	if len(p) > 1 && p[len(p)-1] == '/' {
		return "**/" + p[:len(p)-1] + "/**"
	}

	// Leading star with extension: "*.log" → "**/*.log"
	if len(p) > 1 && p[0] == '*' && p[1] == '.' {
		return "**/" + p
	}

	// Exact name match: match at any depth: ".env" → "**/.env" or exact
	// But if it has no glob chars, keep exact for top-level and add globstar
	if !hasGlobChars(p) {
		return p // exact match at top level is fine
	}

	return p
}

func containsGlobstar(p string) bool {
	return len(p) >= 2 && (p[:2] == "**" || strings.Contains(p, "**/"))
}

func hasGlobChars(p string) bool {
	return strings.ContainsAny(p, "*?[")
}
