package audit

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/r-dson/abox/internal/osutil"
)

// Status represents the outcome of a single audit check.
type Status string

const (
	Pass Status = "pass"
	Fail Status = "fail"
	Warn Status = "warn"
)

var (
	sensitiveWorkspacePaths = []string{".env", ".ssh/id_rsa"}
	secureHomeDirFunc       = osutil.SystemHomeDir
)

// Check represents a single audit finding.
type Check struct {
	Name   string
	Status Status
	Detail string
}

// Result holds the complete audit result.
type Result struct {
	Checks []Check
}

// Run executes all audit checks and returns the result.
func Run(_ context.Context, workdir string) (*Result, error) {
	result := &Result{}

	result.Checks = append(result.Checks, checkWorkdirSafety(workdir))

	sensitiveStatus, sensitivePath := checkSensitiveFiles(workdir)
	result.Checks = append(result.Checks, Check{
		Name:   "sensitive_files",
		Status: sensitiveStatus,
		Detail: sensitivePath,
	})

	return result, nil
}

// CheckWorkdirSafety returns Fail if the workdir resolves to $HOME or /.
func CheckWorkdirSafety(workdir string) Status {
	return checkWorkdirSafety(workdir).Status
}

func checkWorkdirSafety(workdir string) Check {
	check := Check{Name: "workdir_safety", Status: Pass}
	abs, err := filepath.EvalSymlinks(workdir)
	if err != nil {
		check.Status = Fail
		check.Detail = fmt.Sprintf("resolving workspace: %v", err)
		return check
	}

	home, err := secureHomeDirFunc()
	if err != nil {
		check.Status = Fail
		check.Detail = fmt.Sprintf("resolving user home: %v", err)
		return check
	}
	if abs == home {
		check.Status = Fail
		check.Detail = abs
		return check
	}
	if abs == "/" {
		check.Status = Fail
		check.Detail = abs
		return check
	}
	return check
}

// CheckSensitiveFiles returns Warn if known sensitive files exist in workdir.
func CheckSensitiveFiles(workdir string) Status {
	status, _ := checkSensitiveFiles(workdir)
	return status
}

func checkSensitiveFiles(workdir string) (Status, string) {
	for _, name := range sensitiveWorkspacePaths {
		path := filepath.Join(workdir, name)
		if _, err := os.Stat(path); err == nil {
			return Warn, path
		}
	}
	return Pass, ""
}
