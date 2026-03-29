package daemon

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"time"

	"github.com/hatsunemiku3939/jobsd/internal/domain"
)

const maxCapturedOutputBytes = 64 * 1024

type Executor interface {
	Execute(ctx context.Context, command string) ExecutionResult
}

type ExecutionResult struct {
	Status       domain.RunStatus
	StartedAt    time.Time
	FinishedAt   time.Time
	ExitCode     *int
	ErrorMessage *string
	Output       *domain.RunOutput
}

type ShellExecutor struct{}

type capturedStream struct {
	Text      string
	Truncated bool
	ReadError error
	HadOutput bool
}

func (ShellExecutor) Execute(ctx context.Context, command string) ExecutionResult {
	startedAt := time.Now().UTC()
	shell, args := shellCommand(command)

	cmd := exec.CommandContext(ctx, shell, args...)

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		finishedAt := time.Now().UTC()
		return ExecutionResult{
			Status:       domain.RunStatusFailed,
			StartedAt:    startedAt,
			FinishedAt:   finishedAt,
			ErrorMessage: stringPointer(fmt.Sprintf("stdout pipe: %v", err)),
		}
	}

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		finishedAt := time.Now().UTC()
		return ExecutionResult{
			Status:       domain.RunStatusFailed,
			StartedAt:    startedAt,
			FinishedAt:   finishedAt,
			ErrorMessage: stringPointer(fmt.Sprintf("stderr pipe: %v", err)),
		}
	}

	if err := cmd.Start(); err != nil {
		finishedAt := time.Now().UTC()
		return ExecutionResult{
			Status:       domain.RunStatusFailed,
			StartedAt:    startedAt,
			FinishedAt:   finishedAt,
			ErrorMessage: stringPointer(fmt.Sprintf("start command: %v", err)),
		}
	}

	stdoutCh := captureStreamAsync(stdoutPipe)
	stderrCh := captureStreamAsync(stderrPipe)

	stdout := <-stdoutCh
	stderr := <-stderrCh
	waitErr := cmd.Wait()
	finishedAt := time.Now().UTC()

	result := ExecutionResult{
		Status:     domain.RunStatusSucceeded,
		StartedAt:  startedAt,
		FinishedAt: finishedAt,
		ExitCode:   exitCodeFromCommand(cmd, waitErr),
	}

	switch {
	case ctx.Err() != nil:
		result.Status = domain.RunStatusFailed
		result.ErrorMessage = stringPointer(ctx.Err().Error())
	case stdout.ReadError != nil:
		result.Status = domain.RunStatusFailed
		result.ErrorMessage = stringPointer(fmt.Sprintf("stdout read: %v", stdout.ReadError))
	case stderr.ReadError != nil:
		result.Status = domain.RunStatusFailed
		result.ErrorMessage = stringPointer(fmt.Sprintf("stderr read: %v", stderr.ReadError))
	case waitErr == nil:
		if result.ExitCode == nil {
			result.ExitCode = intPointer(0)
		}
	case isExitError(waitErr):
		result.Status = domain.RunStatusFailed
	default:
		result.Status = domain.RunStatusFailed
		if waitErr != nil {
			result.ErrorMessage = stringPointer(fmt.Sprintf("wait command: %v", waitErr))
		}
	}

	if output := buildRunOutput(stdout, stderr, finishedAt); output != nil {
		result.Output = output
	}

	return result
}

func captureStreamAsync(reader io.Reader) <-chan capturedStream {
	resultCh := make(chan capturedStream, 1)
	go func() {
		resultCh <- readCapturedStream(reader)
	}()
	return resultCh
}

func readCapturedStream(reader io.Reader) capturedStream {
	var (
		buffer bytes.Buffer
		result capturedStream
		chunk  = make([]byte, 4096)
	)

	for {
		n, err := reader.Read(chunk)
		if n > 0 {
			result.HadOutput = true
			remaining := maxCapturedOutputBytes - buffer.Len()
			switch {
			case remaining <= 0:
				result.Truncated = true
			case n > remaining:
				_, _ = buffer.Write(chunk[:remaining])
				result.Truncated = true
			default:
				_, _ = buffer.Write(chunk[:n])
			}
		}

		if err == nil {
			continue
		}
		if errors.Is(err, io.EOF) {
			break
		}

		result.ReadError = err
		break
	}

	result.Text = buffer.String()
	return result
}

func buildRunOutput(stdout, stderr capturedStream, finishedAt time.Time) *domain.RunOutput {
	if stdout.Text == "" && stderr.Text == "" && !stdout.Truncated && !stderr.Truncated {
		return nil
	}

	return &domain.RunOutput{
		Stdout:          stdout.Text,
		Stderr:          stderr.Text,
		StdoutTruncated: stdout.Truncated,
		StderrTruncated: stderr.Truncated,
		UpdatedAt:       finishedAt,
	}
}

func exitCodeFromCommand(cmd *exec.Cmd, waitErr error) *int {
	if cmd != nil && cmd.ProcessState != nil {
		exitCode := cmd.ProcessState.ExitCode()
		if exitCode >= 0 {
			return intPointer(exitCode)
		}
	}

	var exitErr *exec.ExitError
	if errors.As(waitErr, &exitErr) {
		exitCode := exitErr.ExitCode()
		if exitCode >= 0 {
			return intPointer(exitCode)
		}
	}

	return nil
}

func isExitError(err error) bool {
	var exitErr *exec.ExitError
	return errors.As(err, &exitErr)
}

func intPointer(value int) *int {
	return &value
}

func stringPointer(value string) *string {
	return &value
}
