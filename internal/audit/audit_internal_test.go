package audit

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

func TestAuditSensitiveFilesPermissionErrorWarns(t *testing.T) {
	oldWalkDir := walkDirFunc
	walkDirFunc = func(root string, fn fs.WalkDirFunc) error {
		return fn(filepath.Join(root, ".aws", "credentials"), nil, fs.ErrPermission)
	}
	defer func() { walkDirFunc = oldWalkDir }()

	result, err := Run(t.Context(), t.TempDir())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	check := findCheck(result, "sensitive_files")
	if check.Status != Warn || !strings.Contains(check.Detail, filepath.Join(".aws", "credentials")) {
		t.Fatalf("sensitive check = %+v, want permission warning detail", check)
	}
}

func TestAuditContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := Run(ctx, t.TempDir())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context canceled", err)
	}
}

func findCheck(result *Result, name string) Check {
	for _, check := range result.Checks {
		if check.Name == name {
			return check
		}
	}
	return Check{}
}

func TestAuditHomeDir_PropagatesErrorsForSecurityChecks(t *testing.T) {
	oldSecureHomeDir := secureHomeDirFunc
	secureHomeDirFunc = func() (string, error) { return "", errors.New("home unavailable") }
	defer func() { secureHomeDirFunc = oldSecureHomeDir }()

	check := checkWorkdirSafety(t.TempDir())
	if check.Status != Fail || !strings.Contains(check.Detail, "resolving user home") {
		t.Fatalf("CheckWorkdirSafety() = %+v, want fail detail resolving user home", check)
	}
}
