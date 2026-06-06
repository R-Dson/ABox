package audit_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/r-dson/abox/internal/audit"
)

func TestRun_ReturnsTypedResult(t *testing.T) {
	result, err := audit.Run(t.Context(), ".")
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if result == nil {
		t.Fatal("Run() returned nil result")
	}
}

func TestResult_HasStatusPerCheck(t *testing.T) {
	result, err := audit.Run(t.Context(), ".")
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if len(result.Checks) == 0 {
		t.Error("Result has no checks")
	}
	for _, check := range result.Checks {
		if check.Name == "" {
			t.Error("check has empty Name")
		}
		if check.Status != audit.Pass && check.Status != audit.Fail && check.Status != audit.Warn {
			t.Errorf("check %q has invalid status %q", check.Name, check.Status)
		}
	}
}

func TestCheckWorkdirSafety(t *testing.T) {
	home, _ := os.UserHomeDir()

	tests := []struct {
		name    string
		workdir string
		want    audit.Status
	}{
		{"tmp passes", "/tmp", audit.Pass},
		{"root fails", "/", audit.Fail},
	}

	// Only test $HOME if it's set and not "/"
	if home != "" && home != "/" {
		tests = append(tests, struct {
			name    string
			workdir string
			want    audit.Status
		}{"home fails", home, audit.Fail})
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := audit.CheckWorkdirSafety(tt.workdir)
			if status != tt.want {
				t.Errorf("CheckWorkdirSafety(%q) = %v, want %v", tt.workdir, status, tt.want)
			}
		})
	}
}

func TestCheckSensitiveFiles(t *testing.T) {
	dir := t.TempDir()

	// Clean dir should pass
	status := audit.CheckSensitiveFiles(dir)
	if status != audit.Pass {
		t.Errorf("clean dir = %v, want Pass", status)
	}

	// Dir with .env should warn
	envFile := filepath.Join(dir, ".env")
	if err := os.WriteFile(envFile, []byte("SECRET=123"), 0o644); err != nil {
		t.Fatal(err)
	}

	status = audit.CheckSensitiveFiles(dir)
	if status != audit.Warn {
		t.Errorf("dir with .env = %v, want Warn", status)
	}
}
