package app

import (
	"errors"
	"fmt"
	"strings"

	"github.com/hatsunemiku3939/jobsd/internal/domain"
	"github.com/hatsunemiku3939/jobsd/internal/output"
	"github.com/hatsunemiku3939/jobsd/internal/sqlite"
	"github.com/spf13/cobra"
)

func newRunCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Inspect job execution history",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	cmd.AddCommand(
		newRunListCommand(),
		newRunGetCommand(),
	)

	return cmd
}

func newRunListCommand() *cobra.Command {
	var (
		instance string
		jobName  string
		status   string
		limit    int
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List recent runs",
		RunE: func(cmd *cobra.Command, args []string) error {
			flags := cmd.Flags()
			if flags.Changed("job") && strings.TrimSpace(jobName) == "" {
				return fmt.Errorf("job must not be empty")
			}

			var statusFilter *domain.RunStatus
			if flags.Changed("status") {
				if strings.TrimSpace(status) == "" {
					return fmt.Errorf("status must not be empty")
				}

				parsedStatus := domain.RunStatus(status)
				if !parsedStatus.IsValid() {
					return fmt.Errorf("invalid run status %q", status)
				}
				statusFilter = &parsedStatus
			}

			if flags.Changed("limit") && limit <= 0 {
				return fmt.Errorf("limit must be greater than zero")
			}

			db, cleanup, err := openInstanceDB(cmd.Context(), instance)
			if err != nil {
				return err
			}
			defer cleanup()

			runs, err := db.Runs.List(cmd.Context(), sqlite.ListRunsFilter{
				JobName: jobName,
				Status:  statusFilter,
				Limit:   limit,
			})
			if err != nil {
				return err
			}

			items := make([]runSummaryOutput, 0, len(runs))
			for _, run := range runs {
				items = append(items, runSummaryFromDomain(run))
			}

			return printRunList(cmd, items)
		},
	}

	cmd.Flags().StringVar(&instance, "instance", "", "Instance name")
	cmd.Flags().StringVar(&jobName, "job", "", "Filter runs by job name")
	cmd.Flags().StringVar(&status, "status", "", "Filter runs by status")
	cmd.Flags().IntVar(&limit, "limit", 20, "Maximum number of runs to return")
	_ = cmd.MarkFlagRequired("instance")

	return cmd
}

func newRunGetCommand() *cobra.Command {
	var (
		instance string
		runID    int64
	)

	cmd := &cobra.Command{
		Use:   "get",
		Short: "Show the details of one run",
		RunE: func(cmd *cobra.Command, args []string) error {
			if runID <= 0 {
				return fmt.Errorf("run-id must be greater than zero")
			}

			db, cleanup, err := openInstanceDB(cmd.Context(), instance)
			if err != nil {
				return err
			}
			defer cleanup()

			run, err := db.Runs.Get(cmd.Context(), runID)
			if err != nil {
				return runLookupError(runID, err)
			}

			return printRunDetail(cmd, runDetailFromDomain(run))
		},
	}

	cmd.Flags().StringVar(&instance, "instance", "", "Instance name")
	cmd.Flags().Int64Var(&runID, "run-id", 0, "Run identifier")
	_ = cmd.MarkFlagRequired("instance")
	_ = cmd.MarkFlagRequired("run-id")

	return cmd
}

func runLookupError(runID int64, err error) error {
	if errors.Is(err, sqlite.ErrRunNotFound) {
		return fmt.Errorf("run %d not found", runID)
	}

	return err
}

func printRunList(cmd *cobra.Command, items []runSummaryOutput) error {
	format, err := commandOutputFormat(cmd)
	if err != nil {
		return err
	}

	printer := output.New(cmd.OutOrStdout(), format)
	if format == output.FormatJSON {
		return printer.PrintJSON(items)
	}

	rows := make([][]string, 0, len(items))
	for _, item := range items {
		rows = append(rows, []string{
			fmt.Sprintf("%d", item.ID),
			item.Job,
			item.Trigger,
			item.Status,
			item.QueuedAt,
			stringValue(item.StartedAt),
			stringValue(item.FinishedAt),
			item.Duration,
		})
	}

	return printer.PrintTable(
		[]string{"ID", "JOB", "TRIGGER", "STATUS", "QUEUED_AT", "STARTED_AT", "FINISHED_AT", "DURATION"},
		rows,
	)
}

func printRunDetail(cmd *cobra.Command, item runDetailOutput) error {
	format, err := commandOutputFormat(cmd)
	if err != nil {
		return err
	}

	printer := output.New(cmd.OutOrStdout(), format)
	if format == output.FormatJSON {
		return printer.PrintJSON(item)
	}

	return printer.PrintTable(
		[]string{"FIELD", "VALUE"},
		fieldValueRows(
			fieldValue{Field: "ID", Value: fmt.Sprintf("%d", item.ID)},
			fieldValue{Field: "JOB", Value: item.Job},
			fieldValue{Field: "JOB_ID", Value: fmt.Sprintf("%d", item.JobID)},
			fieldValue{Field: "TRIGGER_TYPE", Value: item.TriggerType},
			fieldValue{Field: "STATUS", Value: item.Status},
			fieldValue{Field: "SCHEDULED_FOR", Value: stringValue(item.ScheduledFor)},
			fieldValue{Field: "QUEUED_AT", Value: item.QueuedAt},
			fieldValue{Field: "STARTED_AT", Value: stringValue(item.StartedAt)},
			fieldValue{Field: "FINISHED_AT", Value: stringValue(item.FinishedAt)},
			fieldValue{Field: "DURATION", Value: item.Duration},
			fieldValue{Field: "EXIT_CODE", Value: intPtrString(item.ExitCode)},
			fieldValue{Field: "ERROR_MESSAGE", Value: stringValue(item.ErrorMessage)},
			fieldValue{Field: "RUNNER_ID", Value: stringValue(item.RunnerID)},
			fieldValue{Field: "STDOUT_TRUNCATED", Value: boolString(item.StdoutTruncated)},
			fieldValue{Field: "STDERR_TRUNCATED", Value: boolString(item.StderrTruncated)},
			fieldValue{Field: "STDOUT_PREVIEW", Value: item.StdoutPreview},
			fieldValue{Field: "STDERR_PREVIEW", Value: item.StderrPreview},
			fieldValue{Field: "OUTPUT_UPDATED_AT", Value: stringValue(item.OutputUpdatedAt)},
		),
	)
}
