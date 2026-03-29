package app

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestSchedulerCommandStructure(t *testing.T) {
	cmd := newSchedulerCommand(BuildInfo{})

	children := cmd.Commands()
	gotNames := make([]string, 0, len(children))
	hidden := map[string]bool{}
	for _, child := range children {
		gotNames = append(gotNames, child.Name())
		hidden[child.Name()] = child.Hidden
	}

	slices.Sort(gotNames)
	wantNames := []string{"ping", "serve", "start", "status", "stop"}
	if !slices.Equal(gotNames, wantNames) {
		t.Fatalf("command names = %#v, want %#v", gotNames, wantNames)
	}
	if !hidden["serve"] {
		t.Fatal("serve command Hidden = false, want true")
	}
}

func TestSchedulerStartRequiresFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "missing instance",
			args: []string{"scheduler", "start", "--port", "8080"},
			want: `required flag(s) "instance" not set`,
		},
		{
			name: "missing port",
			args: []string{"scheduler", "start", "--instance", "dev"},
			want: `required flag(s) "port" not set`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := NewRootCommand(BuildInfo{Version: "v1.0.0"}, &bytes.Buffer{}, &bytes.Buffer{})
			cmd.SetArgs(tt.args)

			err := cmd.Execute()
			if err == nil {
				t.Fatal("Execute() error = nil, want error")
			}
			if err.Error() != tt.want {
				t.Fatalf("Execute() error = %q, want %q", err.Error(), tt.want)
			}
		})
	}
}

func TestSchedulerStartReexecsServeMode(t *testing.T) {
	originalStartServeProcess := startServeProcess
	t.Cleanup(func() {
		startServeProcess = originalStartServeProcess
	})

	var gotExecutable string
	var gotArgs []string
	startServeProcess = func(ctx context.Context, executable string, args []string) error {
		gotExecutable = executable
		gotArgs = append([]string(nil), args...)
		return nil
	}

	cmd := NewRootCommand(BuildInfo{Version: "v1.0.0"}, &bytes.Buffer{}, &bytes.Buffer{})
	cmd.SetArgs([]string{"scheduler", "start", "--instance", "dev", "--port", "8080"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if gotExecutable == "" {
		t.Fatal("startServeProcess executable = empty, want non-empty")
	}

	wantArgs := []string{"scheduler", "serve", "--instance", "dev", "--port", "8080"}
	if !slices.Equal(gotArgs, wantArgs) {
		t.Fatalf("startServeProcess args = %#v, want %#v", gotArgs, wantArgs)
	}
}

func TestSchedulerCommandShowsHelp(t *testing.T) {
	stdout := &bytes.Buffer{}
	cmd := NewRootCommand(BuildInfo{Version: "v1.0.0"}, stdout, &bytes.Buffer{})
	cmd.SetArgs([]string{"scheduler"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(stdout.String(), "Manage scheduler lifecycle commands") {
		t.Fatalf("stdout = %q, want scheduler help output", stdout.String())
	}
}

func TestSchedulerStartSurfacesProcessError(t *testing.T) {
	originalStartServeProcess := startServeProcess
	t.Cleanup(func() {
		startServeProcess = originalStartServeProcess
	})

	startServeProcess = func(ctx context.Context, executable string, args []string) error {
		return errors.New("boom")
	}

	cmd := NewRootCommand(BuildInfo{Version: "v1.0.0"}, &bytes.Buffer{}, &bytes.Buffer{})
	cmd.SetArgs([]string{"scheduler", "start", "--instance", "dev", "--port", "8080"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
	if got, want := err.Error(), "start scheduler daemon: boom"; got != want {
		t.Fatalf("Execute() error = %q, want %q", got, want)
	}
}
