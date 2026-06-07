package exclusion

import (
	"os"
	"strings"
	"testing"
)

func TestReadOnlyCloseIgnoresAreExplicit(t *testing.T) {
	data, err := os.ReadFile("matcher.go")
	if err != nil {
		t.Fatalf("reading matcher.go: %v", err)
	}
	for _, forbidden := range []string{"defer response.Body.Close()", "defer f.Close()"} {
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("matcher.go contains %q; use explicit ignored-close closure", forbidden)
		}
	}
}

func TestMergePatternsDoesNotMutateBase(t *testing.T) {
	base := make([]string, 1, 2)
	base[0] = "base"
	additional := []string{"extra"}

	got := mergePatterns(base, additional)
	if len(got) != 2 || got[0] != "base" || got[1] != "extra" {
		t.Fatalf("mergePatterns() = %v, want [base extra]", got)
	}
	if len(base) != 1 || base[0] != "base" {
		t.Fatalf("base slice changed = %v", base)
	}
	if leaked := base[:cap(base)][1]; leaked != "" {
		t.Fatalf("base backing array was mutated at spare capacity: %q", leaked)
	}
}
