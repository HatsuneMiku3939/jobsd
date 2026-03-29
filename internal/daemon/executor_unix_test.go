//go:build !windows

package daemon

import (
	"strconv"
	"strings"
	"testing"
)

func TestShellCommandUnix(t *testing.T) {
	shell, args := shellCommand("echo hi")

	if shell != "sh" {
		t.Fatalf("shell = %q, want %q", shell, "sh")
	}
	if len(args) != 2 || args[0] != "-lc" || args[1] != "echo hi" {
		t.Fatalf("args = %#v, want [-lc echo hi]", args)
	}
}

func helperCommand(t *testing.T, args ...string) string {
	t.Helper()

	parts := helperEnvironmentAssignments(args)
	parts = append(parts,
		unixShellQuote(testBinaryPath(t)),
		unixShellQuote("-test.run=TestExecutorHelperProcess"),
	)

	return strings.Join(parts, " ")
}

func testBinaryPath(t *testing.T) string {
	t.Helper()
	return osExecutablePath(t)
}

func unixShellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}

func helperEnvironmentAssignments(args []string) []string {
	assignments := []string{"GO_WANT_EXECUTOR_HELPER=1"}
	for index, arg := range args {
		assignments = append(assignments, "GO_EXECUTOR_ARG"+strconv.Itoa(index)+"="+unixShellQuote(arg))
	}
	if len(args) > 0 {
		assignments = append(assignments, "GO_EXECUTOR_MODE="+unixShellQuote(args[0]))
	}
	return assignments
}
