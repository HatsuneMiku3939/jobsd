//go:build !windows

package daemon

import (
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

	parts := []string{
		unixShellQuote(testBinaryPath(t)),
		unixShellQuote("-test.run=TestExecutorHelperProcess"),
		unixShellQuote("--"),
	}
	for _, arg := range args {
		parts = append(parts, unixShellQuote(arg))
	}

	return strings.Join(parts, " ")
}

func testBinaryPath(t *testing.T) string {
	t.Helper()
	return osExecutablePath(t)
}

func unixShellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}
