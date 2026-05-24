package main

import (
	"context"
	"fmt"
	"os"

	"github.com/r-dson/abox/internal/cli"
)

var version = "dev"

func main() {
	root := cli.NewRootCmd(version)
	if err := root.ExecuteContext(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "abx: %v\n", err)
		os.Exit(1)
	}
}
