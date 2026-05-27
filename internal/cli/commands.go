package cli

import (
	"github.com/spf13/cobra"
)

// InitDataCommands initializes all data manipulation commands
func InitDataCommands(root *cobra.Command) {
	root.AddCommand(InitPutCommand())
	root.AddCommand(InitGetCommand())
	root.AddCommand(InitDeleteCommand())
	root.AddCommand(InitListCommand())
	root.AddCommand(InitCopyCommand())
	root.AddCommand(InitMoveCommand())
	root.AddCommand(InitRefreshCommand())
	root.AddCommand(InitBrowseCommand())
}
