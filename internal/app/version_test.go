package app

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestVersionCommandOutputs(t *testing.T) {
	info := BuildInfo{
		Version:   "v1.2.3",
		Commit:    "abc1234",
		BuildDate: "2025-03-29T00:00:00Z",
	}

	t.Run("table", func(t *testing.T) {
		stdout, err := executeCommandWithBuildInfo(t, info, "version")
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		const want = "" +
			"FIELD       VALUE\n" +
			"VERSION     v1.2.3\n" +
			"COMMIT      abc1234\n" +
			"BUILD_DATE  2025-03-29T00:00:00Z\n"
		if stdout != want {
			t.Fatalf("version table output = %q, want %q", stdout, want)
		}
	})

	t.Run("json", func(t *testing.T) {
		stdout, err := executeCommandWithBuildInfo(t, info, "--output", "json", "version")
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		var payload versionOutput
		if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
			t.Fatalf("Unmarshal() error = %v", err)
		}
		if payload.Version != info.Version || payload.Commit != info.Commit || payload.BuildDate != info.BuildDate {
			t.Fatalf("payload = %#v, want %#v", payload, versionOutput{
				Version:   info.Version,
				Commit:    info.Commit,
				BuildDate: info.BuildDate,
			})
		}
	})
}

func TestVersionHelpIncludesJobsdExamples(t *testing.T) {
	stdout, err := executeCommandWithBuildInfo(t, BuildInfo{Version: "v1.0.0"}, "version", "--help")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	for _, want := range []string{
		"jobsd version",
		"jobsd --output json version",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout = %q, want substring %q", stdout, want)
		}
	}
}
