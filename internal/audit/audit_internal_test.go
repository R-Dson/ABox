package audit

import (
	"errors"
	"strings"
	"testing"
)

func TestAuditHomeDir_PropagatesErrorsForSecurityChecks(t *testing.T) {
	oldSecureHomeDir := secureHomeDirFunc
	secureHomeDirFunc = func() (string, error) { return "", errors.New("home unavailable") }
	defer func() { secureHomeDirFunc = oldSecureHomeDir }()

	check := checkWorkdirSafety(t.TempDir())
	if check.Status != Fail || !strings.Contains(check.Detail, "resolving user home") {
		t.Fatalf("CheckWorkdirSafety() = %+v, want fail detail resolving user home", check)
	}
}
