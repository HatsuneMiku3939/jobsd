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

func TestPlaceholderCommandsReturnNotImplemented(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "scheduler", args: []string{"scheduler"}, want: "scheduler command is not implemented"},
		{name: "job", args: []string{"job"}, want: "job command is not implemented"},
		{name: "run", args: []string{"run"}, want: "run command is not implemented"},
		{name: "version", args: []string{"version"}, want: "version command is not implemented"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := NewRootCommand(BuildInfo{}, &bytes.Buffer{}, &bytes.Buffer{})
			cmd.SetArgs(tt.args)

			err := cmd.Execute()
			if err == nil {
				t.Fatal("Execute() error = nil, want error")
			}
			if err.Error() != tt.want {
				t.Fatalf("Execute() error = %v, want %q", err, tt.want)
			}
		})
	}
}
