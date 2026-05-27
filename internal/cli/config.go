package cli

import (
	"fmt"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/yachiko/clerk/internal/config"
)

var (
	configGetCmd  *cobra.Command
	configSetCmd  *cobra.Command
	configListCmd *cobra.Command
)

// InitConfigCommands initializes configuration commands
func InitConfigCommands(rootCmd *cobra.Command) {
	configGetCmd = &cobra.Command{
		Use:   "get <key>",
		Short: "Get a configuration value",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, err := config.NewManager()
			if err != nil {
				return err
			}

			value, err := mgr.GetValue(args[0])
			if err != nil {
				return err
			}

			fmt.Println(value)
			return nil
		},
	}

	configSetCmd = &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set a configuration value",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, err := config.NewManager()
			if err != nil {
				return err
			}

			if err := mgr.SetValue(args[0], args[1]); err != nil {
				return err
			}

			if err := mgr.Save(); err != nil {
				return err
			}

			color.Green("Configuration updated: %s = %s", args[0], args[1])
			return nil
		},
	}

	configListCmd = &cobra.Command{
		Use:   "list",
		Short: "List all configuration values",
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, err := config.NewManager()
			if err != nil {
				return err
			}

			bold := color.New(color.Bold)
			for _, key := range mgr.ListKeys() {
				value, _ := mgr.GetValue(key)
				_, _ = bold.Printf("%s: ", key)
				fmt.Println(value)
			}
			return nil
		},
	}

	configCmd := &cobra.Command{
		Use:   "config",
		Short: "Manage clerk configuration",
		Long:  `Get, set, or list configuration options for the clerk CLI tool.`,
	}

	configCmd.AddCommand(configGetCmd)
	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configListCmd)

	rootCmd.AddCommand(configCmd)
}
