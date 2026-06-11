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
	oldCommit := commit
	defer func() {
		newRootCmdFunc = oldNewRootCmd
		commit = oldCommit
	}()
	commit = "abc123"

	newRootCmdFunc = func(info cli.VersionInfo) *cobra.Command {
		if info.Commit != commit {
			t.Fatalf("VersionInfo = %+v, want commit=%s", info, commit)
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
