package main

import (
	"errors"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

func TestMainUsesSignalNotifyContext(t *testing.T) {
	oldNewRootCmd := newRootCmdFunc
	defer func() { newRootCmdFunc = oldNewRootCmd }()

	started := make(chan struct{})
	newRootCmdFunc = func(string) *cobra.Command {
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
