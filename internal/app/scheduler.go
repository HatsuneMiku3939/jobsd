package app

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"

	"github.com/hatsunemiku3939/jobsd/internal/config"
	"github.com/hatsunemiku3939/jobsd/internal/daemon"
	"github.com/spf13/cobra"
)

var startServeProcess = func(ctx context.Context, executable string, args []string) error {
	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.SysProcAttr = newDetachedSysProcAttr()
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Start()
}

func newSchedulerCommand(info BuildInfo) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "scheduler",
		Short: "Manage scheduler lifecycle commands",
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
	var port int

	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start a scheduler daemon for an instance",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := config.ResolvePaths(instance)
			if err != nil {
				return err
			}
			if port <= 0 || port > 65535 {
				return fmt.Errorf("port must be between 1 and 65535")
			}

			executable, err := os.Executable()
			if err != nil {
				return fmt.Errorf("resolve executable path: %w", err)
			}

			serveArgs := []string{
				"scheduler",
				"serve",
				"--instance", instance,
				"--port", fmt.Sprintf("%d", port),
			}
			if err := startServeProcess(cmd.Context(), executable, serveArgs); err != nil {
				return fmt.Errorf("start scheduler daemon: %w", err)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&instance, "instance", "", "Instance name")
	cmd.Flags().IntVar(&port, "port", 0, "Scheduler port")
	_ = cmd.MarkFlagRequired("instance")
	_ = cmd.MarkFlagRequired("port")

	return cmd
}

func newSchedulerServeCommand(info BuildInfo) *cobra.Command {
	var instance string
	var port int

	cmd := &cobra.Command{
		Use:    "serve",
		Short:  "Run the internal scheduler daemon serve mode",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			paths, err := config.ResolvePaths(instance)
			if err != nil {
				return err
			}
			if port <= 0 || port > 65535 {
				return fmt.Errorf("port must be between 1 and 65535")
			}

			logger := slog.New(slog.NewTextHandler(io.Discard, nil))
			return daemon.Serve(cmd.Context(), daemon.ServeOptions{
				Instance: instance,
				Port:     port,
				Paths:    paths,
				Version:  info.Version,
				Logger:   logger,
			})
		},
	}

	cmd.Flags().StringVar(&instance, "instance", "", "Instance name")
	cmd.Flags().IntVar(&port, "port", 0, "Scheduler port")
	_ = cmd.MarkFlagRequired("instance")
	_ = cmd.MarkFlagRequired("port")

	return cmd
}

func newSchedulerStatusCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show scheduler status for an instance",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("scheduler status command is not implemented")
		},
	}
}

func newSchedulerStopCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop a scheduler daemon for an instance",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("scheduler stop command is not implemented")
		},
	}
}

func newSchedulerPingCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "ping",
		Short: "Check scheduler health for an instance",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("scheduler ping command is not implemented")
		},
	}
}
