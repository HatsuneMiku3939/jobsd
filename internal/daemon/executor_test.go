package daemon

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hatsunemiku3939/jobsd/internal/domain"
)

func TestShellExecutorExecute(t *testing.T) {
	tests := []struct {
		name          string
		command       string
		timeout       time.Duration
		wantStatus    domain.RunStatus
		wantExitCode  *int
		checkExitCode bool
		wantError     *string
		wantOutput    bool
		assertResult  func(t *testing.T, result ExecutionResult)
	}{
		{
			name:          "successful execution",
			command:       helperCommand(t, "stdout", "hello"),
			wantStatus:    domain.RunStatusSucceeded,
			wantExitCode:  intPointer(0),
			checkExitCode: true,
			wantOutput:    true,
			assertResult: func(t *testing.T, result ExecutionResult) {
				t.Helper()
				if result.Output == nil {
					t.Fatal("Output = nil, want populated output")
				}
				if result.Output.Stdout != "hello" {
					t.Fatalf("Output.Stdout = %q, want %q", result.Output.Stdout, "hello")
				}
				if result.Output.Stderr != "" {
					t.Fatalf("Output.Stderr = %q, want empty", result.Output.Stderr)
				}
				if !result.Output.UpdatedAt.Equal(result.FinishedAt) {
					t.Fatalf("Output.UpdatedAt = %v, want %v", result.Output.UpdatedAt, result.FinishedAt)
				}
			},
		},
		{
			name:          "non zero exit code",
			command:       helperCommand(t, "stderr-exit", "boom", "7"),
			wantStatus:    domain.RunStatusFailed,
			wantExitCode:  intPointer(7),
			checkExitCode: true,
			wantOutput:    true,
			assertResult: func(t *testing.T, result ExecutionResult) {
				t.Helper()
				if result.ErrorMessage != nil {
					t.Fatalf("ErrorMessage = %q, want nil", *result.ErrorMessage)
				}
				if result.Output == nil {
					t.Fatal("Output = nil, want populated output")
				}
				if result.Output.Stderr != "boom" {
					t.Fatalf("Output.Stderr = %q, want %q", result.Output.Stderr, "boom")
				}
			},
		},
		{
			name:          "stdout truncation",
			command:       helperCommand(t, "stdout-repeat", strconv.Itoa(maxCapturedOutputBytes+17), "a"),
			wantStatus:    domain.RunStatusSucceeded,
			wantExitCode:  intPointer(0),
			checkExitCode: true,
			wantOutput:    true,
			assertResult: func(t *testing.T, result ExecutionResult) {
				t.Helper()
				if result.Output == nil {
					t.Fatal("Output = nil, want populated output")
				}
				if len([]byte(result.Output.Stdout)) != maxCapturedOutputBytes {
					t.Fatalf("len(Output.Stdout) = %d, want %d", len([]byte(result.Output.Stdout)), maxCapturedOutputBytes)
				}
				if !result.Output.StdoutTruncated {
					t.Fatal("Output.StdoutTruncated = false, want true")
				}
				if result.Output.StderrTruncated {
					t.Fatal("Output.StderrTruncated = true, want false")
				}
			},
		},
		{
			name:          "stderr truncation",
			command:       helperCommand(t, "stderr-repeat", strconv.Itoa(maxCapturedOutputBytes+17), "b"),
			wantStatus:    domain.RunStatusSucceeded,
			wantExitCode:  intPointer(0),
			checkExitCode: true,
			wantOutput:    true,
			assertResult: func(t *testing.T, result ExecutionResult) {
				t.Helper()
				if result.Output == nil {
					t.Fatal("Output = nil, want populated output")
				}
				if len([]byte(result.Output.Stderr)) != maxCapturedOutputBytes {
					t.Fatalf("len(Output.Stderr) = %d, want %d", len([]byte(result.Output.Stderr)), maxCapturedOutputBytes)
				}
				if !result.Output.StderrTruncated {
					t.Fatal("Output.StderrTruncated = false, want true")
				}
				if result.Output.StdoutTruncated {
					t.Fatal("Output.StdoutTruncated = true, want false")
				}
			},
		},
		{
			name:          "dual stream independence",
			command:       helperCommand(t, "mixed-repeat", strconv.Itoa(maxCapturedOutputBytes+9), "x", strconv.Itoa(maxCapturedOutputBytes+11), "y"),
			wantStatus:    domain.RunStatusSucceeded,
			wantExitCode:  intPointer(0),
			checkExitCode: true,
			wantOutput:    true,
			assertResult: func(t *testing.T, result ExecutionResult) {
				t.Helper()
				if result.Output == nil {
					t.Fatal("Output = nil, want populated output")
				}
				if len([]byte(result.Output.Stdout)) != maxCapturedOutputBytes {
					t.Fatalf("len(Output.Stdout) = %d, want %d", len([]byte(result.Output.Stdout)), maxCapturedOutputBytes)
				}
				if len([]byte(result.Output.Stderr)) != maxCapturedOutputBytes {
					t.Fatalf("len(Output.Stderr) = %d, want %d", len([]byte(result.Output.Stderr)), maxCapturedOutputBytes)
				}
				if !result.Output.StdoutTruncated || !result.Output.StderrTruncated {
					t.Fatalf("Output truncation = %#v, want both streams truncated", result.Output)
				}
			},
		},
		{
			name:          "context cancellation preserves partial output",
			command:       helperCommand(t, "sleep", "1000", "begin"),
			timeout:       50 * time.Millisecond,
			wantStatus:    domain.RunStatusFailed,
			wantExitCode:  nil,
			checkExitCode: false,
			wantError:     stringPointer(context.DeadlineExceeded.Error()),
			wantOutput:    true,
			assertResult: func(t *testing.T, result ExecutionResult) {
				t.Helper()
				if result.Output == nil {
					t.Fatal("Output = nil, want partial output")
				}
				if result.Output.Stdout != "begin" {
					t.Fatalf("Output.Stdout = %q, want %q", result.Output.Stdout, "begin")
				}
			},
		},
		{
			name:          "empty output success",
			command:       helperCommand(t, "noop"),
			wantStatus:    domain.RunStatusSucceeded,
			wantExitCode:  intPointer(0),
			checkExitCode: true,
			wantOutput:    false,
			assertResult: func(t *testing.T, result ExecutionResult) {
				t.Helper()
				if result.Output != nil {
					t.Fatalf("Output = %#v, want nil", result.Output)
				}
			},
		},
	}

	executor := ShellExecutor{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if tt.timeout > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, tt.timeout)
				defer cancel()
			}

			result := executor.Execute(ctx, tt.command)

			if result.Status != tt.wantStatus {
				t.Fatalf("Status = %q, want %q", result.Status, tt.wantStatus)
			}
			if tt.checkExitCode {
				assertIntPtrEqual(t, "ExitCode", result.ExitCode, tt.wantExitCode)
			}
			assertStringPtrEqual(t, "ErrorMessage", result.ErrorMessage, tt.wantError)
			if result.StartedAt.IsZero() {
				t.Fatal("StartedAt = zero, want timestamp")
			}
			if result.FinishedAt.IsZero() {
				t.Fatal("FinishedAt = zero, want timestamp")
			}
			if result.FinishedAt.Before(result.StartedAt) {
				t.Fatalf("FinishedAt = %v, want >= %v", result.FinishedAt, result.StartedAt)
			}
			if (result.Output != nil) != tt.wantOutput {
				t.Fatalf("Output presence = %t, want %t", result.Output != nil, tt.wantOutput)
			}

			tt.assertResult(t, result)
		})
	}
}

func TestExecutorHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_EXECUTOR_HELPER") != "1" {
		return
	}

	mode := os.Getenv("GO_EXECUTOR_MODE")
	arg1 := os.Getenv("GO_EXECUTOR_ARG1")
	arg2 := os.Getenv("GO_EXECUTOR_ARG2")
	arg3 := os.Getenv("GO_EXECUTOR_ARG3")
	arg4 := os.Getenv("GO_EXECUTOR_ARG4")

	switch mode {
	case "noop":
		os.Exit(0)
	case "stdout":
		if _, err := os.Stdout.WriteString(arg1); err != nil {
			fmt.Fprint(os.Stderr, err)
			os.Exit(2)
		}
		os.Exit(0)
	case "stderr":
		if _, err := os.Stderr.WriteString(arg1); err != nil {
			os.Exit(2)
		}
		os.Exit(0)
	case "stdout-repeat":
		writeRepeatedString(os.Stdout, arg2, arg1)
		os.Exit(0)
	case "stderr-repeat":
		writeRepeatedString(os.Stderr, arg2, arg1)
		os.Exit(0)
	case "mixed":
		if _, err := os.Stdout.WriteString(arg1); err != nil {
			fmt.Fprint(os.Stderr, err)
			os.Exit(2)
		}
		if _, err := os.Stderr.WriteString(arg2); err != nil {
			os.Exit(2)
		}
		os.Exit(0)
	case "mixed-repeat":
		writeRepeatedString(os.Stdout, arg2, arg1)
		writeRepeatedString(os.Stderr, arg4, arg3)
		os.Exit(0)
	case "stderr-exit":
		if _, err := os.Stderr.WriteString(arg1); err != nil {
			os.Exit(2)
		}
		code, err := strconv.Atoi(arg2)
		if err != nil {
			os.Exit(2)
		}
		os.Exit(code)
	case "sleep":
		if arg2 != "" {
			if _, err := os.Stdout.WriteString(arg2); err != nil {
				fmt.Fprint(os.Stderr, err)
				os.Exit(2)
			}
		}
		delayMS, err := strconv.Atoi(arg1)
		if err != nil {
			os.Exit(2)
		}
		time.Sleep(time.Duration(delayMS) * time.Millisecond)
		os.Exit(0)
	default:
		os.Exit(2)
	}
}

func writeRepeatedString(file *os.File, value string, countRaw string) {
	count, err := strconv.Atoi(countRaw)
	if err != nil {
		os.Exit(2)
	}
	if _, err := file.WriteString(strings.Repeat(value, count)); err != nil {
		os.Exit(2)
	}
}

func assertIntPtrEqual(t *testing.T, field string, got, want *int) {
	t.Helper()

	switch {
	case got == nil && want == nil:
		return
	case got == nil || want == nil:
		t.Fatalf("%s = %v, want %v", field, got, want)
	case *got != *want:
		t.Fatalf("%s = %d, want %d", field, *got, *want)
	}
}

func assertStringPtrEqual(t *testing.T, field string, got, want *string) {
	t.Helper()

	switch {
	case got == nil && want == nil:
		return
	case got == nil || want == nil:
		t.Fatalf("%s = %v, want %v", field, got, want)
	case *got != *want:
		t.Fatalf("%s = %q, want %q", field, *got, *want)
	}
}
