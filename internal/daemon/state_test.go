package daemon

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hatsunemiku3939/jobsd/internal/domain"
)

func TestStateRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime", "state.json")
	want := domain.SchedulerState{
		Instance:  "dev",
		PID:       1234,
		Port:      8080,
		Token:     "token-123",
		DBPath:    "/tmp/jobs.db",
		StartedAt: time.Date(2025, 4, 1, 12, 0, 0, 0, time.UTC),
		Version:   "v1.0.0",
	}

	if err := WriteState(path, want); err != nil {
		t.Fatalf("WriteState() error = %v", err)
	}

	got, err := ReadState(path)
	if err != nil {
		t.Fatalf("ReadState() error = %v", err)
	}
	if got != want {
		t.Fatalf("ReadState() = %#v, want %#v", got, want)
	}
}

func TestReadStateErrors(t *testing.T) {
	t.Run("missing file", func(t *testing.T) {
		_, err := ReadState(filepath.Join(t.TempDir(), "missing.json"))
		if !errors.Is(err, ErrStateNotFound) {
			t.Fatalf("ReadState() error = %v, want %v", err, ErrStateNotFound)
		}
	})

	t.Run("malformed json", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "state.json")
		if err := os.WriteFile(path, []byte(`{`), 0o600); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}

		_, err := ReadState(path)
		if !errors.Is(err, ErrStateCorrupt) {
			t.Fatalf("ReadState() error = %v, want %v", err, ErrStateCorrupt)
		}
	})

	t.Run("missing required fields", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "state.json")
		if err := os.WriteFile(path, []byte(`{"instance":"dev"}`), 0o600); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}

		_, err := ReadState(path)
		if !errors.Is(err, ErrStateCorrupt) {
			t.Fatalf("ReadState() error = %v, want %v", err, ErrStateCorrupt)
		}
	})
}

func TestRemoveStateIgnoresMissingFile(t *testing.T) {
	if err := RemoveState(filepath.Join(t.TempDir(), "missing.json")); err != nil {
		t.Fatalf("RemoveState() error = %v", err)
	}
}
