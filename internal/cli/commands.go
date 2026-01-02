package cli

import (
	"github.com/spf13/cobra"
)

// InitDataCommands initializes all data manipulation commands
func InitDataCommands(root *cobra.Command) {
	// Initialize and register data commands
	root.AddCommand(InitPutCommand())
	root.AddCommand(InitGetCommand())
	root.AddCommand(InitDeleteCommand())
	root.AddCommand(InitListCommand())
	root.AddCommand(InitCopyCommand())
	root.AddCommand(InitMoveCommand())
	root.AddCommand(InitRefreshCommand())
	root.AddCommand(InitBrowseCommand())
}

// Exported command variables (to be set during initialization)
var (
	putCmd     *cobra.Command
	getCmd     *cobra.Command
	deleteCmd  *cobra.Command
	listCmd    *cobra.Command
	copyCmd    *cobra.Command
	moveCmd    *cobra.Command
	refreshCmd *cobra.Command
	browseCmd  *cobra.Command
)
