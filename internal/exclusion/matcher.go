package exclusion

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
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
func NewMatcher(patterns []string) *Matcher {
	return &Matcher{patterns: patterns}
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

// BuildMatcher composes patterns from three sources:
//  1. Hardcoded security patterns (always applied)
//  2. Local .abxignore in the workspace
//  3. Remote URL patterns (fetched over HTTPS)
func BuildMatcher(ctx context.Context, workdir, remoteURL string) (*Matcher, error) {
	patterns := HardcodedPatterns()

	if local, err := loadLocalIgnore(workdir); err == nil {
		patterns = mergePatterns(patterns, local)
	}

	if remoteURL != "" {
		if remote, err := fetchRemotePatterns(ctx, remoteURL); err != nil {
			slog.WarnContext(ctx, "remote patterns unavailable, continuing without",
				"url", remoteURL, "error", err)
		} else {
			patterns = mergePatterns(patterns, remote)
		}
	}

	return &Matcher{patterns: patterns}, nil
}

func loadLocalIgnore(workdir string) ([]string, error) {
	path := filepath.Join(workdir, ".abxignore")
	f, err := os.Open(path)
	if err != nil {
		return nil, err
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
	return patterns, scanner.Err()
}

func fetchRemotePatterns(_ context.Context, url string) ([]string, error) {
	// TODO: implement HTTP fetch in a later task
	return nil, fmt.Errorf("remote pattern fetch not yet implemented")
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
