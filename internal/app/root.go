package app

import (
	"context"
	"fmt"
	"io"

	"github.com/hatsunemiku3939/jobsd/internal/output"
	"github.com/spf13/cobra"
)

type BuildInfo struct {
	Version   string
	Commit    string
	BuildDate string
}

func Execute(ctx context.Context, stdout, stderr io.Writer, info BuildInfo) error {
	cmd := NewRootCommand(info, stdout, stderr)
	cmd.SetContext(ctx)
	return cmd.Execute()
}

func NewRootCommand(info BuildInfo, stdout, stderr io.Writer) *cobra.Command {
	var outputFormat string

	cmd := &cobra.Command{
		Use:           "jobsd",
		Short:         "Manage instance-scoped local job schedulers",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			_, err := output.ParseFormat(outputFormat)
			return err
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.PersistentFlags().StringVar(&outputFormat, "output", string(output.FormatTable), "Output format: table|json")

	cmd.AddCommand(
		newSchedulerCommand(info),
		newJobCommand(),
		newRunCommand(),
		newVersionCommand(info),
	)

	return cmd
}

func newPlaceholderCommand(use, short string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("%s command is not implemented", use)
		},
	}
}
