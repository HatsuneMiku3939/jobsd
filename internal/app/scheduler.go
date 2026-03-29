package app

import "github.com/spf13/cobra"

func newSchedulerCommand() *cobra.Command {
	return newPlaceholderCommand("scheduler", "Manage scheduler lifecycle commands")
}
