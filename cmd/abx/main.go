package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/r-dson/abox/internal/cli"
)

var (
	version        = "dev"
	commit         = "unknown"
	date           = "unknown"
	newRootCmdFunc = cli.NewRootCmdWithVersion
)

func main() {
	os.Exit(run())
}

func run() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	root := newRootCmdFunc(cli.VersionInfo{Version: version, Commit: commit, Date: date})
	if err := root.ExecuteContext(ctx); err != nil {
		if exitErr, ok := errors.AsType[*cli.ExitError](err); ok {
			return exitErr.Code
		}
		fmt.Fprintf(os.Stderr, "abx: %v\n", err)
		return 1
	}
	return 0
}
