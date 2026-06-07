package audit

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/r-dson/abox/internal/exclusion"
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
	secureHomeDirFunc = osutil.SystemHomeDir
	walkDirFunc       = filepath.WalkDir
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
func Run(ctx context.Context, workdir string) (*Result, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("audit context: %w", err)
	}
	result := &Result{}

	result.Checks = append(result.Checks, checkWorkdirSafety(workdir))

	sensitiveStatus, sensitivePath, err := checkSensitiveFiles(ctx, workdir)
	if err != nil {
		return nil, err
	}
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
	status, _, _ := checkSensitiveFiles(context.Background(), workdir)
	return status
}

func checkSensitiveFiles(ctx context.Context, workdir string) (Status, string, error) {
	if err := ctx.Err(); err != nil {
		return Pass, "", fmt.Errorf("audit context: %w", err)
	}
	matcher := exclusion.NewMatcher(exclusion.HardcodedPatterns())
	var found string
	if err := walkDirFunc(workdir, func(path string, entry os.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("audit context: %w", err)
		}
		if found != "" {
			return nil
		}
		if walkErr != nil {
			if errors.Is(walkErr, fs.ErrPermission) {
				found = path
				return nil
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(workdir, path)
		if err != nil {
			return nil
		}
		if matcher.Match(filepath.ToSlash(rel)) {
			found = path
		}
		return nil
	}); err != nil {
		return Pass, "", fmt.Errorf("walking workspace for sensitive files: %w", err)
	}
	if found != "" {
		return Warn, found, nil
	}
	return Pass, "", nil
}
