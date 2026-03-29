//go:build windows

package daemon

import (
	"strings"
	"testing"
)

func TestShellCommandWindows(t *testing.T) {
	shell, args := shellCommand("echo hi")

	if shell != "cmd" {
		t.Fatalf("shell = %q, want %q", shell, "cmd")
	}
	if len(args) != 2 || args[0] != "/C" || args[1] != "echo hi" {
		t.Fatalf("args = %#v, want [/C echo hi]", args)
	}
}

func helperCommand(t *testing.T, args ...string) string {
	t.Helper()

	parts := []string{
		windowsShellQuote(testBinaryPath(t)),
		windowsShellQuote("-test.run=TestExecutorHelperProcess"),
		windowsShellQuote("--"),
	}
	for _, arg := range args {
		parts = append(parts, windowsShellQuote(arg))
	}

	return `"` + strings.Join(parts, " ") + `"`
}

func testBinaryPath(t *testing.T) string {
	t.Helper()
	return osExecutablePath(t)
}

func windowsShellQuote(value string) string {
	replacer := strings.NewReplacer(`^`, `^^`, `&`, `^&`, `|`, `^|`, `<`, `^<`, `>`, `^>`, `"`, `""`)
	return `"` + replacer.Replace(value) + `"`
}
