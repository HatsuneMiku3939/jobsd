package app

import "github.com/spf13/cobra"

func newRunCommand() *cobra.Command {
	return newPlaceholderCommand("run", "Inspect job execution history")
}
