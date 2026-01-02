package main

import (
	"fmt"
	"os"

	"github.com/yachiko/clerk/internal/cli"
)

var (
	// These are set at compile time via -ldflags
	Version   = "dev"
	Commit    = "unknown"
	BuildTime = "unknown"
)

func main() {
	rootCmd := cli.NewRootCommand(Version, Commit, BuildTime)
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
