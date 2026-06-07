package exclusion

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bmatcuk/doublestar/v4"
)

// Matcher checks paths against a set of exclusion patterns.
type Matcher struct {
	patterns []string
}

const (
	remoteIgnoreMaxBytes = 1 << 20
	remoteIgnoreTimeout  = 10 * time.Second
)

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
func BuildMatcher(ctx context.Context, workdir string) (*Matcher, error) {
	return BuildMatcherWithRemote(ctx, workdir, "")
}

func BuildMatcherWithRemote(ctx context.Context, workdir, excludeURL string) (*Matcher, error) {
	patterns := HardcodedPatterns()

	local, err := loadLocalIgnore(workdir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err == nil {
		patterns = mergePatterns(patterns, normalizePatterns(local))
	}
	if excludeURL != "" {
		remote, err := loadRemoteIgnore(ctx, excludeURL)
		if err != nil {
			return nil, err
		}
		patterns = mergePatterns(patterns, normalizePatterns(remote))
	}

	if err := validatePatterns(patterns); err != nil {
		return nil, err
	}
	return &Matcher{patterns: patterns}, nil
}

func validatePatterns(patterns []string) error {
	for _, pattern := range patterns {
		if !doublestar.ValidatePattern(pattern) {
			return fmt.Errorf("invalid exclusion pattern %q", pattern)
		}
	}
	return nil
}

func loadRemoteIgnore(ctx context.Context, excludeURL string) ([]string, error) {
	parsedURL, err := url.Parse(excludeURL)
	if err != nil {
		return nil, fmt.Errorf("parsing exclude URL: %w", err)
	}
	if parsedURL.Scheme != "https" {
		return nil, fmt.Errorf("exclude URL must use HTTPS")
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, excludeURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating exclude URL request: %w", err)
	}
	client := &http.Client{Timeout: remoteIgnoreTimeout}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("fetching exclude URL: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("fetching exclude URL: unexpected status %s", response.Status)
	}
	body, err := readLimitedRemoteBody(response.Body, remoteIgnoreMaxBytes)
	if err != nil {
		return nil, fmt.Errorf("reading exclude URL: %w", err)
	}
	patterns, err := readIgnorePatterns(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("parsing exclude URL: %w", err)
	}
	return patterns, nil
}

func readLimitedRemoteBody(r io.Reader, maxBytes int64) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(r, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("reading remote ignore body: %w", err)
	}
	if int64(len(body)) > maxBytes {
		return nil, fmt.Errorf("remote ignore body exceeds %d bytes", maxBytes)
	}
	return body, nil
}

func loadLocalIgnore(workdir string) ([]string, error) {
	path := filepath.Join(workdir, ".abxignore")
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening ignore file: %w", err)
	}
	defer func() { _ = f.Close() }()

	return readIgnorePatterns(f)
}

func readIgnorePatterns(r io.Reader) ([]string, error) {
	var patterns []string
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading ignore patterns: %w", err)
	}
	return patterns, nil
}

func mergePatterns(base, additional []string) []string {
	merged := append([]string(nil), base...)
	seen := make(map[string]bool)
	for _, p := range merged {
		seen[p] = true
	}
	for _, p := range additional {
		if !seen[p] {
			merged = append(merged, p)
			seen[p] = true
		}
	}
	return merged
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

	// Exact name match: match at any depth: ".env" → "**/.env".
	if !hasGlobChars(p) {
		return "**/" + p
	}

	return p
}

func containsGlobstar(p string) bool {
	return len(p) >= 2 && (p[:2] == "**" || strings.Contains(p, "**/"))
}

func hasGlobChars(p string) bool {
	return strings.ContainsAny(p, "*?[")
}
