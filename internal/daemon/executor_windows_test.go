//go:build windows

package daemon

import (
	"strconv"
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

	parts := []string{`set "GO_WANT_EXECUTOR_HELPER=1"`}
	for index, arg := range args {
		parts = append(parts, `set "GO_EXECUTOR_ARG`+strconv.Itoa(index)+`=`+arg+`"`)
	}
	if len(args) > 0 {
		parts = append(parts, `set "GO_EXECUTOR_MODE=`+args[0]+`"`)
	}
	parts = append(parts, windowsShellQuote(testBinaryPath(t))+` `+windowsShellQuote("-test.run=TestExecutorHelperProcess"))

	return strings.Join(parts, " && ")
}

func testBinaryPath(t *testing.T) string {
	t.Helper()
	return osExecutablePath(t)
}

func windowsShellQuote(value string) string {
	replacer := strings.NewReplacer(`^`, `^^`, `&`, `^&`, `|`, `^|`, `<`, `^<`, `>`, `^>`, `"`, `""`)
	return `"` + replacer.Replace(value) + `"`
}
