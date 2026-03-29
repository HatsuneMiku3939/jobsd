package app

import (
	"strings"

	"github.com/hatsunemiku3939/jobsd/internal/output"
	"github.com/spf13/cobra"
)

type versionOutput struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildDate string `json:"build_date"`
}

func newVersionCommand(info BuildInfo) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Example: strings.TrimSpace(`
jobsd version
jobsd --output json version`),
		RunE: func(cmd *cobra.Command, args []string) error {
			value := versionOutput{
				Version:   info.Version,
				Commit:    info.Commit,
				BuildDate: info.BuildDate,
			}

			format, err := commandOutputFormat(cmd)
			if err != nil {
				return err
			}

			printer := output.New(cmd.OutOrStdout(), format)
			if format == output.FormatJSON {
				return printer.PrintJSON(value)
			}

			return printer.PrintFields([]output.Field{
				{Name: "VERSION", Value: value.Version},
				{Name: "COMMIT", Value: value.Commit},
				{Name: "BUILD_DATE", Value: value.BuildDate},
			})
		},
	}

	return cmd
}
