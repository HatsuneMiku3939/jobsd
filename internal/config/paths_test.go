package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolvePaths(t *testing.T) {
	t.Run("uses xdg directories when present", func(t *testing.T) {
		t.Setenv("XDG_DATA_HOME", "/tmp/jobsd-data")
		t.Setenv("XDG_RUNTIME_DIR", "/tmp/jobsd-runtime")
		t.Setenv("HOME", "/tmp/home-ignored")

		paths, err := ResolvePaths("demo.instance-1")
		if err != nil {
			t.Fatalf("ResolvePaths() error = %v", err)
		}

		if got, want := paths.DataDir, filepath.Join("/tmp/jobsd-data", "jobsd", "instances", "demo.instance-1"); got != want {
			t.Fatalf("DataDir = %q, want %q", got, want)
		}
		if got, want := paths.RuntimeDir, filepath.Join("/tmp/jobsd-runtime", "jobsd", "demo.instance-1"); got != want {
			t.Fatalf("RuntimeDir = %q, want %q", got, want)
		}
		if got, want := paths.LockPath, filepath.Join("/tmp/jobsd-runtime", "jobsd", "demo.instance-1.lock"); got != want {
			t.Fatalf("LockPath = %q, want %q", got, want)
		}
		if got, want := paths.StatePath, filepath.Join("/tmp/jobsd-runtime", "jobsd", "demo.instance-1", "state.json"); got != want {
			t.Fatalf("StatePath = %q, want %q", got, want)
		}
	})

	t.Run("falls back to home and temp directories", func(t *testing.T) {
		homeDir := t.TempDir()
		t.Setenv("HOME", homeDir)
		t.Setenv("XDG_DATA_HOME", "")
		t.Setenv("XDG_RUNTIME_DIR", "")

		paths, err := ResolvePaths("demo")
		if err != nil {
			t.Fatalf("ResolvePaths() error = %v", err)
		}

		if got, want := paths.DataDir, filepath.Join(homeDir, ".local", "share", "jobsd", "instances", "demo"); got != want {
			t.Fatalf("DataDir = %q, want %q", got, want)
		}

		runtimeBase := filepath.Join(filepath.Clean(filepath.Join(os.TempDir(), "jobsd-"+currentUserID())), "demo")
		if got, want := paths.RuntimeDir, runtimeBase; got != want {
			t.Fatalf("RuntimeDir = %q, want %q", got, want)
		}
		if got, want := paths.DatabasePath, filepath.Join(homeDir, ".local", "share", "jobsd", "instances", "demo", "jobs.db"); got != want {
			t.Fatalf("DatabasePath = %q, want %q", got, want)
		}
	})
}

func TestResolvePathsRejectsInvalidInstanceNames(t *testing.T) {
	tests := []struct {
		name     string
		instance string
	}{
		{name: "empty", instance: ""},
		{name: "slash", instance: "bad/name"},
		{name: "backslash", instance: `bad\name`},
		{name: "space", instance: "bad name"},
		{name: "special", instance: "bad!name"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ResolvePaths(tt.instance); err == nil {
				t.Fatalf("ResolvePaths(%q) error = nil, want error", tt.instance)
			}
		})
	}
}
