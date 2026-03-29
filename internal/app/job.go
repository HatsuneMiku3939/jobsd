package app

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/hatsunemiku3939/jobsd/internal/daemon"
	"github.com/hatsunemiku3939/jobsd/internal/domain"
	"github.com/hatsunemiku3939/jobsd/internal/output"
	"github.com/hatsunemiku3939/jobsd/internal/schedule"
	"github.com/hatsunemiku3939/jobsd/internal/sqlite"
	"github.com/spf13/cobra"
)

func newJobCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "job",
		Short: "Manage jobs within an instance",
		Example: strings.TrimSpace(`
jobsd job list --instance dev
jobsd job get --instance dev --name cleanup
jobsd job run --instance dev --name cleanup`),
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	cmd.AddCommand(
		newJobAddCommand(),
		newJobListCommand(),
		newJobGetCommand(),
		newJobUpdateCommand(),
		newJobDeleteCommand(),
		newJobPauseCommand(),
		newJobResumeCommand(),
		newJobRunCommand(),
	)

	return cmd
}

func newJobAddCommand() *cobra.Command {
	var (
		instance          string
		name              string
		scheduleRaw       string
		command           string
		timezone          string
		disabled          bool
		concurrencyPolicy string
	)

	cmd := &cobra.Command{
		Use:   "add",
		Short: "Create a new job",
		Example: strings.TrimSpace(`
jobsd job add \
  --instance dev \
  --name cleanup \
  --schedule "every 10m" \
  --command "echo cleanup"`),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredValue("name", name); err != nil {
				return err
			}
			if err := validateRequiredValue("schedule", scheduleRaw); err != nil {
				return err
			}
			if err := validateRequiredValue("command", command); err != nil {
				return err
			}

			parsedSchedule, err := schedule.Parse(scheduleRaw)
			if err != nil {
				return err
			}

			normalizedTZ := normalizedTimezone(timezone)
			if _, err := loadLocation(normalizedTZ); err != nil {
				return err
			}

			policy, err := parseConcurrencyPolicy(concurrencyPolicy)
			if err != nil {
				return err
			}

			now := currentTime()
			enabled := !disabled
			var nextRunAt *time.Time
			if enabled {
				nextRunAt, err = computeNextRun(parsedSchedule, normalizedTZ, now)
				if err != nil {
					return fmt.Errorf("compute next run: %w", err)
				}
			}

			db, cleanup, err := openInstanceDB(cmd.Context(), instance)
			if err != nil {
				return err
			}
			defer cleanup()

			job, err := db.Jobs.Create(cmd.Context(), domain.Job{
				Name:              name,
				Command:           command,
				ScheduleKind:      parsedSchedule.Kind,
				ScheduleExpr:      parsedSchedule.Expr,
				Timezone:          normalizedTZ,
				Enabled:           enabled,
				ConcurrencyPolicy: policy,
				NextRunAt:         nextRunAt,
				CreatedAt:         now,
				UpdatedAt:         now,
			})
			if err != nil {
				return err
			}

			return printJobDetail(cmd, jobDetailFromDomain(job))
		},
	}

	cmd.Flags().StringVar(&instance, "instance", "", "Instance name")
	cmd.Flags().StringVar(&name, "name", "", "Job name")
	cmd.Flags().StringVar(&scheduleRaw, "schedule", "", "Schedule expression")
	cmd.Flags().StringVar(&command, "command", "", "Shell command to execute")
	cmd.Flags().StringVar(&timezone, "timezone", "Local", "Timezone for cron schedule evaluation")
	cmd.Flags().BoolVar(&disabled, "disabled", false, "Create the job in a disabled state")
	cmd.Flags().StringVar(&concurrencyPolicy, "concurrency-policy", string(domain.ConcurrencyPolicyForbid), "Concurrency policy: forbid|queue|replace")
	_ = cmd.MarkFlagRequired("instance")
	_ = cmd.MarkFlagRequired("name")
	_ = cmd.MarkFlagRequired("schedule")
	_ = cmd.MarkFlagRequired("command")

	return cmd
}

func newJobListCommand() *cobra.Command {
	var instance string

	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List jobs for an instance",
		Example: "jobsd job list --instance dev",
		RunE: func(cmd *cobra.Command, args []string) error {
			db, cleanup, err := openInstanceDB(cmd.Context(), instance)
			if err != nil {
				return err
			}
			defer cleanup()

			jobs, err := db.Jobs.List(cmd.Context())
			if err != nil {
				return err
			}

			items := make([]jobSummaryOutput, 0, len(jobs))
			for _, job := range jobs {
				items = append(items, jobSummaryFromDomain(job))
			}

			return printJobList(cmd, items)
		},
	}

	cmd.Flags().StringVar(&instance, "instance", "", "Instance name")
	_ = cmd.MarkFlagRequired("instance")

	return cmd
}

func newJobGetCommand() *cobra.Command {
	var (
		instance string
		name     string
	)

	cmd := &cobra.Command{
		Use:     "get",
		Short:   "Show the details of one job",
		Example: "jobsd job get --instance dev --name cleanup",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredValue("name", name); err != nil {
				return err
			}

			db, cleanup, err := openInstanceDB(cmd.Context(), instance)
			if err != nil {
				return err
			}
			defer cleanup()

			job, err := db.Jobs.GetByName(cmd.Context(), name)
			if err != nil {
				return jobLookupError(name, err)
			}

			return printJobDetail(cmd, jobDetailFromDomain(job))
		},
	}

	cmd.Flags().StringVar(&instance, "instance", "", "Instance name")
	cmd.Flags().StringVar(&name, "name", "", "Job name")
	_ = cmd.MarkFlagRequired("instance")
	_ = cmd.MarkFlagRequired("name")

	return cmd
}

func newJobUpdateCommand() *cobra.Command {
	var (
		instance          string
		name              string
		newName           string
		command           string
		scheduleRaw       string
		timezone          string
		concurrencyPolicy string
		enable            bool
		disable           bool
	)

	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update a job definition",
		Example: strings.TrimSpace(`
jobsd job update \
  --instance dev \
  --name cleanup \
  --new-name cleanup-nightly \
  --schedule "every 30m" \
  --timezone UTC \
  --concurrency-policy queue`),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredValue("name", name); err != nil {
				return err
			}

			flags := cmd.Flags()
			nameChanged := flags.Changed("new-name")
			commandChanged := flags.Changed("command")
			scheduleChanged := flags.Changed("schedule")
			timezoneChanged := flags.Changed("timezone")
			policyChanged := flags.Changed("concurrency-policy")
			enableChanged := flags.Changed("enabled")
			disableChanged := flags.Changed("disabled")

			if !nameChanged && !commandChanged && !scheduleChanged && !timezoneChanged && !policyChanged && !enableChanged && !disableChanged {
				return fmt.Errorf("at least one field to update must be provided")
			}
			if enableChanged && disableChanged {
				return fmt.Errorf("--enabled and --disabled cannot be used together")
			}

			var parsedSchedule domain.Schedule
			if nameChanged {
				if err := validateChangedValue("new-name", newName); err != nil {
					return err
				}
			}
			if commandChanged {
				if err := validateChangedValue("command", command); err != nil {
					return err
				}
			}
			if scheduleChanged {
				if err := validateChangedValue("schedule", scheduleRaw); err != nil {
					return err
				}

				var err error
				parsedSchedule, err = schedule.Parse(scheduleRaw)
				if err != nil {
					return err
				}
			}
			if timezoneChanged {
				if err := validateChangedValue("timezone", timezone); err != nil {
					return err
				}
				if _, err := loadLocation(timezone); err != nil {
					return err
				}
			}

			var policy domain.ConcurrencyPolicy
			var err error
			if policyChanged {
				if err := validateChangedValue("concurrency-policy", concurrencyPolicy); err != nil {
					return err
				}

				policy, err = parseConcurrencyPolicy(concurrencyPolicy)
				if err != nil {
					return err
				}
			}

			db, cleanup, err := openInstanceDB(cmd.Context(), instance)
			if err != nil {
				return err
			}
			defer cleanup()

			job, err := db.Jobs.GetByName(cmd.Context(), name)
			if err != nil {
				return jobLookupError(name, err)
			}

			updated := job
			changed := false
			recomputeNextRun := false
			scheduleValueChanged := false
			timezoneValueChanged := false
			enabledValueChanged := false

			if nameChanged {
				updated.Name = newName
				changed = changed || updated.Name != job.Name
			}
			if commandChanged {
				updated.Command = command
				changed = changed || updated.Command != job.Command
			}
			if scheduleChanged {
				updated.ScheduleKind = parsedSchedule.Kind
				updated.ScheduleExpr = parsedSchedule.Expr
				scheduleValueChanged = updated.ScheduleKind != job.ScheduleKind || updated.ScheduleExpr != job.ScheduleExpr
				recomputeNextRun = recomputeNextRun || scheduleValueChanged
				changed = changed || scheduleValueChanged
			}
			if timezoneChanged {
				updated.Timezone = timezone
				timezoneValueChanged = normalizedTimezone(updated.Timezone) != normalizedTimezone(job.Timezone)
				recomputeNextRun = recomputeNextRun || timezoneValueChanged
				changed = changed || timezoneValueChanged
			}
			if policyChanged {
				updated.ConcurrencyPolicy = policy
				changed = changed || updated.ConcurrencyPolicy != job.ConcurrencyPolicy
			}
			if enableChanged {
				updated.Enabled = true
				enabledValueChanged = updated.Enabled != job.Enabled
				recomputeNextRun = recomputeNextRun || enabledValueChanged
				changed = changed || enabledValueChanged
			}
			if disableChanged {
				updated.Enabled = false
				enabledValueChanged = updated.Enabled != job.Enabled
				recomputeNextRun = recomputeNextRun || enabledValueChanged
				changed = changed || enabledValueChanged
			}

			if recomputeNextRun {
				if !updated.Enabled {
					updated.NextRunAt = nil
				} else {
					nextRunAt, err := computeNextRun(domain.Schedule{
						Kind: updated.ScheduleKind,
						Expr: updated.ScheduleExpr,
					}, updated.Timezone, currentTime())
					if err != nil {
						return fmt.Errorf("compute next run: %w", err)
					}
					updated.NextRunAt = nextRunAt
				}
			}

			if !changed && !recomputeNextRun {
				return printJobDetail(cmd, jobDetailFromDomain(job))
			}

			updated.UpdatedAt = currentTime()
			job, err = db.Jobs.Update(cmd.Context(), updated)
			if err != nil {
				return jobLookupError(name, err)
			}

			return printJobDetail(cmd, jobDetailFromDomain(job))
		},
	}

	cmd.Flags().StringVar(&instance, "instance", "", "Instance name")
	cmd.Flags().StringVar(&name, "name", "", "Job name")
	cmd.Flags().StringVar(&newName, "new-name", "", "New job name")
	cmd.Flags().StringVar(&command, "command", "", "Updated shell command")
	cmd.Flags().StringVar(&scheduleRaw, "schedule", "", "Updated schedule expression")
	cmd.Flags().StringVar(&timezone, "timezone", "", "Updated timezone")
	cmd.Flags().StringVar(&concurrencyPolicy, "concurrency-policy", "", "Updated concurrency policy")
	cmd.Flags().BoolVar(&enable, "enabled", false, "Enable the job")
	cmd.Flags().BoolVar(&disable, "disabled", false, "Disable the job")
	_ = cmd.MarkFlagRequired("instance")
	_ = cmd.MarkFlagRequired("name")

	return cmd
}

func newJobDeleteCommand() *cobra.Command {
	var (
		instance string
		name     string
	)

	cmd := &cobra.Command{
		Use:     "delete",
		Short:   "Delete a job",
		Example: "jobsd job delete --instance dev --name cleanup",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredValue("name", name); err != nil {
				return err
			}

			db, cleanup, err := openInstanceDB(cmd.Context(), instance)
			if err != nil {
				return err
			}
			defer cleanup()

			if err := db.Jobs.DeleteByName(cmd.Context(), name); err != nil {
				return jobLookupError(name, err)
			}

			return printDeleteResult(cmd, deleteResultOutput{
				Name:    name,
				Deleted: true,
			})
		},
	}

	cmd.Flags().StringVar(&instance, "instance", "", "Instance name")
	cmd.Flags().StringVar(&name, "name", "", "Job name")
	_ = cmd.MarkFlagRequired("instance")
	_ = cmd.MarkFlagRequired("name")

	return cmd
}

func newJobPauseCommand() *cobra.Command {
	var (
		instance string
		name     string
	)

	cmd := &cobra.Command{
		Use:     "pause",
		Short:   "Disable scheduled execution for a job",
		Example: "jobsd job pause --instance dev --name cleanup",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredValue("name", name); err != nil {
				return err
			}

			db, cleanup, err := openInstanceDB(cmd.Context(), instance)
			if err != nil {
				return err
			}
			defer cleanup()

			job, err := db.Jobs.GetByName(cmd.Context(), name)
			if err != nil {
				return jobLookupError(name, err)
			}

			if !job.Enabled {
				return printJobDetail(cmd, jobDetailFromDomain(job))
			}

			job.Enabled = false
			job.NextRunAt = nil
			job.UpdatedAt = currentTime()

			job, err = db.Jobs.Update(cmd.Context(), job)
			if err != nil {
				return jobLookupError(name, err)
			}

			return printJobDetail(cmd, jobDetailFromDomain(job))
		},
	}

	cmd.Flags().StringVar(&instance, "instance", "", "Instance name")
	cmd.Flags().StringVar(&name, "name", "", "Job name")
	_ = cmd.MarkFlagRequired("instance")
	_ = cmd.MarkFlagRequired("name")

	return cmd
}

func newJobResumeCommand() *cobra.Command {
	var (
		instance string
		name     string
	)

	cmd := &cobra.Command{
		Use:     "resume",
		Short:   "Re-enable scheduled execution for a job",
		Example: "jobsd job resume --instance dev --name cleanup",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredValue("name", name); err != nil {
				return err
			}

			db, cleanup, err := openInstanceDB(cmd.Context(), instance)
			if err != nil {
				return err
			}
			defer cleanup()

			job, err := db.Jobs.GetByName(cmd.Context(), name)
			if err != nil {
				return jobLookupError(name, err)
			}

			if job.Enabled {
				return printJobDetail(cmd, jobDetailFromDomain(job))
			}

			nextRunAt, err := computeNextRun(domain.Schedule{
				Kind: job.ScheduleKind,
				Expr: job.ScheduleExpr,
			}, job.Timezone, currentTime())
			if err != nil {
				return fmt.Errorf("compute next run: %w", err)
			}

			job.Enabled = true
			job.NextRunAt = nextRunAt
			job.UpdatedAt = currentTime()

			job, err = db.Jobs.Update(cmd.Context(), job)
			if err != nil {
				return jobLookupError(name, err)
			}

			return printJobDetail(cmd, jobDetailFromDomain(job))
		},
	}

	cmd.Flags().StringVar(&instance, "instance", "", "Instance name")
	cmd.Flags().StringVar(&name, "name", "", "Job name")
	_ = cmd.MarkFlagRequired("instance")
	_ = cmd.MarkFlagRequired("name")

	return cmd
}

func newJobRunCommand() *cobra.Command {
	var (
		instance string
		name     string
	)

	cmd := &cobra.Command{
		Use:     "run",
		Short:   "Trigger a job immediately",
		Example: "jobsd job run --instance dev --name cleanup",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredValue("name", name); err != nil {
				return err
			}

			db, cleanup, err := openInstanceDB(cmd.Context(), instance)
			if err != nil {
				return err
			}
			defer cleanup()

			job, err := db.Jobs.GetByName(cmd.Context(), name)
			if err != nil {
				return jobLookupError(name, err)
			}

			run, err := daemon.EnqueueManualWithPolicy(cmd.Context(), db.Runs, job, currentTime())
			if err != nil {
				if errors.Is(err, daemon.ErrRunConflict) {
					return fmt.Errorf("job %q already has a pending or running run", name)
				}
				return err
			}

			return printRunEnqueue(cmd, runEnqueueFromDomain(job.Name, run))
		},
	}

	cmd.Flags().StringVar(&instance, "instance", "", "Instance name")
	cmd.Flags().StringVar(&name, "name", "", "Job name")
	_ = cmd.MarkFlagRequired("instance")
	_ = cmd.MarkFlagRequired("name")

	return cmd
}

func validateRequiredValue(flagName string, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s must not be empty", flagName)
	}

	return nil
}

func validateChangedValue(flagName string, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s must not be empty", flagName)
	}

	return nil
}

func parseConcurrencyPolicy(raw string) (domain.ConcurrencyPolicy, error) {
	policy := domain.ConcurrencyPolicy(raw)
	if !policy.IsValid() {
		return "", fmt.Errorf("invalid concurrency policy %q", raw)
	}

	return policy, nil
}

func jobLookupError(name string, err error) error {
	if errors.Is(err, sqlite.ErrJobNotFound) {
		return fmt.Errorf("job %q not found", name)
	}

	return err
}

func printJobList(cmd *cobra.Command, items []jobSummaryOutput) error {
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
			item.Name,
			boolString(item.Enabled),
			item.Schedule,
			item.Timezone,
			item.ConcurrencyPolicy,
			stringValue(item.NextRunAt),
			stringValue(item.LastRunAt),
			stringValue(item.LastRunStatus),
		})
	}

	return printer.PrintTable(
		[]string{"NAME", "ENABLED", "SCHEDULE", "TIMEZONE", "POLICY", "NEXT_RUN_AT", "LAST_RUN_AT", "LAST_RUN_STATUS"},
		rows,
	)
}

func printJobDetail(cmd *cobra.Command, item jobDetailOutput) error {
	format, err := commandOutputFormat(cmd)
	if err != nil {
		return err
	}

	printer := output.New(cmd.OutOrStdout(), format)
	if format == output.FormatJSON {
		return printer.PrintJSON(item)
	}

	return printer.PrintFields([]output.Field{
		{Name: "ID", Value: fmt.Sprintf("%d", item.ID)},
		{Name: "NAME", Value: item.Name},
		{Name: "COMMAND", Value: item.Command},
		{Name: "SCHEDULE", Value: item.Schedule},
		{Name: "TIMEZONE", Value: item.Timezone},
		{Name: "ENABLED", Value: boolString(item.Enabled)},
		{Name: "CONCURRENCY_POLICY", Value: item.ConcurrencyPolicy},
		{Name: "NEXT_RUN_AT", Value: stringValue(item.NextRunAt)},
		{Name: "LAST_RUN_AT", Value: stringValue(item.LastRunAt)},
		{Name: "LAST_RUN_STATUS", Value: stringValue(item.LastRunStatus)},
		{Name: "CREATED_AT", Value: item.CreatedAt},
		{Name: "UPDATED_AT", Value: item.UpdatedAt},
	})
}

func printDeleteResult(cmd *cobra.Command, item deleteResultOutput) error {
	format, err := commandOutputFormat(cmd)
	if err != nil {
		return err
	}

	printer := output.New(cmd.OutOrStdout(), format)
	if format == output.FormatJSON {
		return printer.PrintJSON(item)
	}

	return printer.PrintFields([]output.Field{
		{Name: "NAME", Value: item.Name},
		{Name: "DELETED", Value: boolString(item.Deleted)},
	})
}

func printRunEnqueue(cmd *cobra.Command, item runEnqueueOutput) error {
	format, err := commandOutputFormat(cmd)
	if err != nil {
		return err
	}

	printer := output.New(cmd.OutOrStdout(), format)
	if format == output.FormatJSON {
		return printer.PrintJSON(item)
	}

	return printer.PrintFields([]output.Field{
		{Name: "RUN_ID", Value: fmt.Sprintf("%d", item.RunID)},
		{Name: "JOB", Value: item.Job},
		{Name: "STATUS", Value: item.Status},
		{Name: "TRIGGER_TYPE", Value: item.TriggerType},
		{Name: "QUEUED_AT", Value: item.QueuedAt},
	})
}
