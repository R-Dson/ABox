package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/r-dson/abox/internal/cli"
)

var version = "dev"

func main() {
	root := cli.NewRootCmd(version)
	if err := root.ExecuteContext(context.Background()); err != nil {
		var exitErr *cli.ExitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.Code)
		}
		fmt.Fprintf(os.Stderr, "abx: %v\n", err)
		os.Exit(1)
	}
}
