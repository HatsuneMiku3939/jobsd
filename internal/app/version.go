package app

import "github.com/spf13/cobra"

func newVersionCommand(info BuildInfo) *cobra.Command {
	cmd := newPlaceholderCommand("version", "Print version information")
	cmd.Annotations = map[string]string{
		"version":    info.Version,
		"commit":     info.Commit,
		"build_date": info.BuildDate,
	}

	return cmd
}
