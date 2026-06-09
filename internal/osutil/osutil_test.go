package osutil_test

import (
	"testing"

	"github.com/r-dson/abox/internal/osutil"
)

func TestHomeDirUsesEnvironmentFirst(t *testing.T) {
	t.Setenv("HOME", "/test/home")
	if got := osutil.HomeDir(); got != "/test/home" {
		t.Fatalf("HomeDir() = %q, want /test/home", got)
	}
}

func TestHomeDirFallsBackToOSUserHome(t *testing.T) {
	t.Setenv("HOME", "")
	if got := osutil.HomeDir(); got == "" {
		t.Fatal("HomeDir() returned empty fallback")
	}
}
