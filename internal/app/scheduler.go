package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/hatsunemiku3939/jobsd/internal/config"
	"github.com/hatsunemiku3939/jobsd/internal/daemon"
	"github.com/hatsunemiku3939/jobsd/internal/domain"
	"github.com/hatsunemiku3939/jobsd/internal/output"
	"github.com/spf13/cobra"
)

var startServeProcess = func(ctx context.Context, executable string, args []string) error {
	cmd := exec.CommandContext(ctx, executable, args...)
	configureDetachedProcess(cmd)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Start()
}

type schedulerOutput struct {
	Instance  string                 `json:"instance"`
	Status    domain.SchedulerStatus `json:"status"`
	PID       int                    `json:"pid,omitempty"`
	Port      int                    `json:"port,omitempty"`
	DBPath    string                 `json:"db_path,omitempty"`
	StartedAt string                 `json:"started_at,omitempty"`
	Version   string                 `json:"version,omitempty"`
	Reason    string                 `json:"reason,omitempty"`
}

func newSchedulerCommand(info BuildInfo) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "scheduler",
		Short: "Manage scheduler lifecycle commands",
		Example: strings.TrimSpace(`
jobsd scheduler start --instance dev
jobsd scheduler status --instance dev
jobsd scheduler stop --instance dev`),
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	cmd.AddCommand(
		newSchedulerStartCommand(),
		newSchedulerStatusCommand(),
		newSchedulerStopCommand(),
		newSchedulerPingCommand(),
		newSchedulerServeCommand(info),
	)

	return cmd
}

func newSchedulerStartCommand() *cobra.Command {
	var instance string

	cmd := &cobra.Command{
		Use:     "start",
		Short:   "Start a scheduler daemon for an instance",
		Example: "jobsd scheduler start --instance dev",
		RunE: func(cmd *cobra.Command, args []string) error {
			paths, err := config.ResolvePaths(instance)
			if err != nil {
				return err
			}

			inspection, err := inspectScheduler(cmd.Context(), instance)
			if err != nil {
				return err
			}
			if inspection.Status == domain.SchedulerStatusRunning {
				return fmt.Errorf("instance %q is already running", instance)
			}

			executable, err := os.Executable()
			if err != nil {
				return fmt.Errorf("resolve executable path: %w", err)
			}

			serveArgs := []string{
				"scheduler",
				"serve",
				"--instance", instance,
			}
			if err := startServeProcess(cmd.Context(), executable, serveArgs); err != nil {
				return fmt.Errorf("start scheduler daemon: %w", err)
			}

			readyInspection, err := waitForSchedulerStart(cmd.Context(), instance, paths.StatePath)
			if err != nil {
				return err
			}

			return printSchedulerOutput(cmd, schedulerOutputFromInspection(instance, readyInspection))
		},
	}

	cmd.Flags().StringVar(&instance, "instance", "", "Instance name")
	_ = cmd.MarkFlagRequired("instance")

	return cmd
}

func newSchedulerServeCommand(info BuildInfo) *cobra.Command {
	var instance string

	cmd := &cobra.Command{
		Use:    "serve",
		Short:  "Run the internal scheduler daemon serve mode",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			paths, err := config.ResolvePaths(instance)
			if err != nil {
				return err
			}

			logger := slog.New(slog.NewTextHandler(io.Discard, nil))
			return daemon.Serve(cmd.Context(), daemon.ServeOptions{
				Instance: instance,
				Paths:    paths,
				Version:  info.Version,
				Logger:   logger,
			})
		},
	}

	cmd.Flags().StringVar(&instance, "instance", "", "Instance name")
	_ = cmd.MarkFlagRequired("instance")

	return cmd
}

func newSchedulerStatusCommand() *cobra.Command {
	var instance string

	cmd := &cobra.Command{
		Use:     "status",
		Short:   "Show scheduler status for an instance",
		Example: "jobsd scheduler status --instance dev",
		RunE: func(cmd *cobra.Command, args []string) error {
			inspection, err := inspectScheduler(cmd.Context(), instance)
			if err != nil {
				return err
			}

			return printSchedulerOutput(cmd, schedulerOutputFromInspection(instance, inspection))
		},
	}

	cmd.Flags().StringVar(&instance, "instance", "", "Instance name")
	_ = cmd.MarkFlagRequired("instance")

	return cmd
}

func newSchedulerStopCommand() *cobra.Command {
	var instance string

	cmd := &cobra.Command{
		Use:     "stop",
		Short:   "Stop a scheduler daemon for an instance",
		Example: "jobsd scheduler stop --instance dev",
		RunE: func(cmd *cobra.Command, args []string) error {
			inspection, err := inspectScheduler(cmd.Context(), instance)
			if err != nil {
				return err
			}

			switch inspection.Status {
			case domain.SchedulerStatusStopped:
				return fmt.Errorf("instance %q is not running", instance)
			case domain.SchedulerStatusStale:
				return fmt.Errorf("instance %q is stale: %s", instance, inspectionReason(inspection))
			}
			if inspection.State == nil {
				return fmt.Errorf("instance %q state is unavailable", instance)
			}

			client := newSchedulerClient()
			if err := client.shutdown(cmd.Context(), *inspection.State); err != nil {
				return fmt.Errorf("request scheduler shutdown: %w", err)
			}
			if err := waitForSchedulerStop(cmd.Context(), inspection.Paths.StatePath, *inspection.State); err != nil {
				return err
			}

			return printSchedulerOutput(cmd, schedulerOutput{
				Instance: instance,
				Status:   domain.SchedulerStatusStopped,
			})
		},
	}

	cmd.Flags().StringVar(&instance, "instance", "", "Instance name")
	_ = cmd.MarkFlagRequired("instance")

	return cmd
}

func newSchedulerPingCommand() *cobra.Command {
	var instance string

	cmd := &cobra.Command{
		Use:     "ping",
		Short:   "Check scheduler health for an instance",
		Example: "jobsd scheduler ping --instance dev",
		RunE: func(cmd *cobra.Command, args []string) error {
			inspection, err := inspectScheduler(cmd.Context(), instance)
			if err != nil {
				return err
			}

			switch inspection.Status {
			case domain.SchedulerStatusStopped:
				return fmt.Errorf("instance %q is stopped", instance)
			case domain.SchedulerStatusStale:
				return fmt.Errorf("instance %q is stale: %s", instance, inspectionReason(inspection))
			}
			if inspection.Ping == nil {
				return fmt.Errorf("instance %q ping response is unavailable", instance)
			}

			return printSchedulerOutput(cmd, schedulerOutput{
				Instance:  inspection.Ping.Instance,
				Status:    inspection.Ping.Status,
				PID:       inspection.Ping.PID,
				Port:      inspection.Ping.Port,
				StartedAt: inspection.Ping.StartedAt,
				Version:   inspection.Ping.Version,
			})
		},
	}

	cmd.Flags().StringVar(&instance, "instance", "", "Instance name")
	_ = cmd.MarkFlagRequired("instance")

	return cmd
}

func waitForSchedulerStart(ctx context.Context, instance string, statePath string) (schedulerInspection, error) {
	deadline := time.Now().Add(schedulerStartupTimeout)
	var lastErr string

	for time.Now().Before(deadline) {
		inspection, err := inspectScheduler(ctx, instance)
		if err != nil {
			lastErr = err.Error()
		} else {
			switch inspection.Status {
			case domain.SchedulerStatusRunning:
				return inspection, nil
			case domain.SchedulerStatusStale:
				lastErr = inspectionReason(inspection)
			case domain.SchedulerStatusStopped:
				if _, err := daemon.ReadState(statePath); err == nil {
					lastErr = "scheduler state exists but daemon is not ready yet"
				}
			}
		}

		select {
		case <-ctx.Done():
			return schedulerInspection{}, ctx.Err()
		case <-time.After(schedulerPollInterval):
		}
	}

	if lastErr != "" {
		return schedulerInspection{}, fmt.Errorf(
			"scheduler did not become ready within %s: %s",
			schedulerStartupTimeout,
			lastErr,
		)
	}

	return schedulerInspection{}, fmt.Errorf(
		"scheduler did not become ready within %s",
		schedulerStartupTimeout,
	)
}

func waitForSchedulerStop(ctx context.Context, statePath string, state domain.SchedulerState) error {
	client := newSchedulerClient()
	deadline := time.Now().Add(schedulerStopTimeout)
	var lastErr string

	for time.Now().Before(deadline) {
		_, stateErr := daemon.ReadState(statePath)
		_, pingErr := client.ping(ctx, state)
		if errors.Is(stateErr, daemon.ErrStateNotFound) && pingErr != nil {
			return nil
		}
		if stateErr != nil && !errors.Is(stateErr, daemon.ErrStateNotFound) {
			lastErr = stateErr.Error()
		} else if pingErr != nil {
			lastErr = pingErr.Error()
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(schedulerPollInterval):
		}
	}

	if lastErr != "" {
		return fmt.Errorf("scheduler did not stop within %s: %s", schedulerStopTimeout, lastErr)
	}

	return fmt.Errorf("scheduler did not stop within %s", schedulerStopTimeout)
}

func inspectionReason(inspection schedulerInspection) string {
	if inspection.Reason != "" {
		return inspection.Reason
	}

	return "scheduler control api is unreachable"
}

func schedulerOutputFromInspection(instance string, inspection schedulerInspection) schedulerOutput {
	output := schedulerOutput{
		Instance: instance,
		Status:   inspection.Status,
		Reason:   inspection.Reason,
	}

	if inspection.State != nil {
		output.PID = inspection.State.PID
		output.Port = inspection.State.Port
		output.DBPath = inspection.State.DBPath
		output.StartedAt = inspection.State.StartedAt.UTC().Format(time.RFC3339)
		output.Version = inspection.State.Version
	}
	if inspection.SchedulerInfo != nil {
		output.Instance = inspection.SchedulerInfo.Instance
		output.Status = inspection.SchedulerInfo.Status
		output.PID = inspection.SchedulerInfo.PID
		output.Port = inspection.SchedulerInfo.Port
		output.DBPath = inspection.SchedulerInfo.DBPath
		output.StartedAt = inspection.SchedulerInfo.StartedAt
		output.Version = inspection.SchedulerInfo.Version
		output.Reason = ""
	}

	return output
}

func printSchedulerOutput(cmd *cobra.Command, value schedulerOutput) error {
	format, err := commandOutputFormat(cmd)
	if err != nil {
		return err
	}

	printer := output.New(cmd.OutOrStdout(), format)
	if format == output.FormatJSON {
		return printer.PrintJSON(value)
	}

	return printer.PrintTable(
		[]string{"INSTANCE", "STATUS", "PID", "PORT", "DB_PATH", "STARTED_AT", "VERSION", "REASON"},
		[][]string{{
			value.Instance,
			string(value.Status),
			intString(value.PID),
			intString(value.Port),
			value.DBPath,
			value.StartedAt,
			value.Version,
			value.Reason,
		}},
	)
}

func commandOutputFormat(cmd *cobra.Command) (output.Format, error) {
	value, err := cmd.Flags().GetString("output")
	if err != nil {
		return "", fmt.Errorf("read output flag: %w", err)
	}

	return output.ParseFormat(value)
}

func intString(value int) string {
	if value == 0 {
		return ""
	}

	return fmt.Sprintf("%d", value)
}
