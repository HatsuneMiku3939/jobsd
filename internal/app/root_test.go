package app

import (
	"bytes"
	"slices"
	"strings"
	"testing"
)

func TestNewRootCommandStructure(t *testing.T) {
	cmd := NewRootCommand(BuildInfo{}, &bytes.Buffer{}, &bytes.Buffer{})

	if got := cmd.Name(); got != "jobsd" {
		t.Fatalf("Name() = %q, want %q", got, "jobsd")
	}

	children := cmd.Commands()
	gotNames := make([]string, 0, len(children))
	for _, child := range children {
		gotNames = append(gotNames, child.Name())
	}

	wantNames := []string{"job", "run", "scheduler", "version"}
	slices.Sort(gotNames)
	if !slices.Equal(gotNames, wantNames) {
		t.Fatalf("command names = %#v, want %#v", gotNames, wantNames)
	}
}

func TestRootCommandRejectsInvalidOutputFormat(t *testing.T) {
	cmd := NewRootCommand(BuildInfo{}, &bytes.Buffer{}, &bytes.Buffer{})
	cmd.SetArgs([]string{"--output", "yaml", "scheduler"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "unsupported output format") {
		t.Fatalf("Execute() error = %q, want unsupported output format", err)
	}
}

func TestRootHelpIncludesJobsdExamples(t *testing.T) {
	cmd := NewRootCommand(BuildInfo{Version: "v1.0.0"}, &bytes.Buffer{}, &bytes.Buffer{})
	cmd.SetArgs([]string{"--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	output := cmd.OutOrStdout().(*bytes.Buffer).String()
	for _, want := range []string{
		"jobsd scheduler start --instance dev --port 8080",
		"jobsd job add --instance dev --name cleanup --schedule \"every 10m\" --command \"echo cleanup\"",
		"jobsd version",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("help output missing %q: %q", want, output)
		}
	}
}
