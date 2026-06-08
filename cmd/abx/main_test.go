package main

import (
	"errors"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/r-dson/abox/internal/cli"
	"github.com/spf13/cobra"
)

func TestRunPassesVersionMetadata(t *testing.T) {
	oldNewRootCmd := newRootCmdFunc
	oldVersion := version
	oldCommit := commit
	oldDate := date
	defer func() {
		newRootCmdFunc = oldNewRootCmd
		version = oldVersion
		commit = oldCommit
		date = oldDate
	}()
	version = "1.2.3"
	commit = "abc123"
	date = "2026-06-08T00:00:00Z"

	newRootCmdFunc = func(info cli.VersionInfo) *cobra.Command {
		if info.Version != version || info.Commit != commit || info.Date != date {
			t.Fatalf("VersionInfo = %+v, want version/commit/date", info)
		}
		return &cobra.Command{Use: "test"}
	}

	if code := run(); code != 0 {
		t.Fatalf("run() exit code = %d, want 0", code)
	}
}

func TestMainUsesSignalNotifyContext(t *testing.T) {
	oldNewRootCmd := newRootCmdFunc
	defer func() { newRootCmdFunc = oldNewRootCmd }()

	started := make(chan struct{})
	newRootCmdFunc = func(cli.VersionInfo) *cobra.Command {
		return &cobra.Command{
			Use: "test",
			RunE: func(cmd *cobra.Command, _ []string) error {
				close(started)
				select {
				case <-cmd.Context().Done():
					return nil
				case <-time.After(2 * time.Second):
					return errors.New("context was not canceled")
				}
			},
		}
	}

	go func() {
		<-started
		_ = syscall.Kill(os.Getpid(), syscall.SIGINT)
	}()

	if code := run(); code != 0 {
		t.Fatalf("run() exit code = %d, want 0", code)
	}
}
