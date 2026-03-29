package app

import "github.com/spf13/cobra"

func newJobCommand() *cobra.Command {
	return newPlaceholderCommand("job", "Manage jobs within an instance")
}
