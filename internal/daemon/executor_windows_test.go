//go:build windows

package daemon

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"unicode/utf16"
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

	switch args[0] {
	case "noop":
		return powershellCommand("exit 0")
	case "stdout":
		return powershellCommand(fmt.Sprintf("[Console]::Out.Write(%s)", powerShellLiteral(args[1])))
	case "stderr":
		return powershellCommand(fmt.Sprintf("[Console]::Error.Write(%s)", powerShellLiteral(args[1])))
	case "stdout-repeat":
		count, err := strconv.Atoi(args[1])
		if err != nil {
			t.Fatalf("Atoi() count error = %v", err)
		}
		return powershellCommand(fmt.Sprintf("[Console]::Out.Write((%s) * %d)", powerShellLiteral(args[2]), count))
	case "stderr-repeat":
		count, err := strconv.Atoi(args[1])
		if err != nil {
			t.Fatalf("Atoi() count error = %v", err)
		}
		return powershellCommand(fmt.Sprintf("[Console]::Error.Write((%s) * %d)", powerShellLiteral(args[2]), count))
	case "mixed-repeat":
		stdoutCount, err := strconv.Atoi(args[1])
		if err != nil {
			t.Fatalf("Atoi() stdout count error = %v", err)
		}
		stderrCount, err := strconv.Atoi(args[3])
		if err != nil {
			t.Fatalf("Atoi() stderr count error = %v", err)
		}
		return powershellCommand(fmt.Sprintf(
			"[Console]::Out.Write((%s) * %d); [Console]::Error.Write((%s) * %d)",
			powerShellLiteral(args[2]),
			stdoutCount,
			powerShellLiteral(args[4]),
			stderrCount,
		))
	case "stderr-exit":
		return powershellCommand(fmt.Sprintf(
			"[Console]::Error.Write(%s); exit %s",
			powerShellLiteral(args[1]),
			args[2],
		))
	case "sleep":
		delayMS, err := strconv.Atoi(args[1])
		if err != nil {
			t.Fatalf("Atoi() delay error = %v", err)
		}
		return powershellCommand(fmt.Sprintf(
			"[Console]::Out.Write(%s); Start-Sleep -Milliseconds %d",
			powerShellLiteral(args[2]),
			delayMS,
		))
	default:
		t.Fatalf("unsupported helper mode %q", args[0])
	}

	return ""
}

func testBinaryPath(t *testing.T) string {
	t.Helper()
	return osExecutablePath(t)
}

func windowsShellQuote(value string) string {
	replacer := strings.NewReplacer(`^`, `^^`, `&`, `^&`, `|`, `^|`, `<`, `^<`, `>`, `^>`, `"`, `""`)
	return `"` + replacer.Replace(value) + `"`
}

func powershellCommand(script string) string {
	return "powershell -NoProfile -EncodedCommand " + encodePowerShellScript(script)
}

func powerShellLiteral(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func encodePowerShellScript(script string) string {
	encoded := utf16.Encode([]rune(script))
	bytes := make([]byte, 0, len(encoded)*2)
	for _, value := range encoded {
		bytes = append(bytes, byte(value), byte(value>>8))
	}
	return base64.StdEncoding.EncodeToString(bytes)
}
