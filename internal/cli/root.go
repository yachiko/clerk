package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// GlobalOptions holds flags shared across all commands
type GlobalOptions struct {
	Region  string
	Profile string
	Output  string // "plain" or "json"
	Verbose bool
}

var globalOpts GlobalOptions
var rootCmd *cobra.Command

// NewRootCommand creates and returns the root command
func NewRootCommand(version, commit, buildTime string) *cobra.Command {
	rootCmd = &cobra.Command{
		Use:           "clerk",
		Short:         "Manage AWS Systems Manager Parameter Store with ease",
		Long:          `clerk - A CLI tool for managing AWS Systems Manager Parameter Store secrets and configurations`,
		SilenceErrors: true,
		SilenceUsage:  true,
	}

	// Add version command
	rootCmd.AddCommand(newVersionCommand(version, commit, buildTime))

	// Add global flags
	// Note: region default is empty to allow config file to override
	rootCmd.PersistentFlags().StringVar(&globalOpts.Region, "region", "", "AWS region (defaults to config file region)")
	rootCmd.PersistentFlags().StringVar(&globalOpts.Profile, "profile", "", "AWS profile to use")
	rootCmd.PersistentFlags().StringVar(&globalOpts.Output, "output", "plain", "Output format (plain or json)")
	rootCmd.PersistentFlags().BoolVar(&globalOpts.Verbose, "verbose", false, "Enable verbose output")

	// Initialize config commands
	InitConfigCommands(rootCmd)

	// Initialize data commands
	InitDataCommands(rootCmd)

	return rootCmd
}

// newVersionCommand creates the version subcommand
func newVersionCommand(version, commit, buildTime string) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("clerk version %s\n", version)
			fmt.Printf("commit: %s\n", commit)
			fmt.Printf("built: %s\n", buildTime)
		},
	}
}

// Exit terminates the program with the given exit code and optional message
func Exit(code int, msg string) {
	if msg != "" {
		fmt.Fprintf(os.Stderr, "%s\n", msg)
	}
	os.Exit(code)
}
