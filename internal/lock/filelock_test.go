package lock

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestAcquireRejectsDuplicateLock(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "demo.lock")

	first, err := Acquire(lockPath)
	if err != nil {
		t.Fatalf("Acquire() first error = %v", err)
	}
	t.Cleanup(func() {
		if err := first.Release(); err != nil {
			t.Fatalf("Release() first error = %v", err)
		}
	})

	second, err := Acquire(lockPath)
	if second != nil {
		t.Fatal("Acquire() second lock = non-nil, want nil")
	}
	if !errors.Is(err, ErrAlreadyLocked) {
		t.Fatalf("Acquire() second error = %v, want %v", err, ErrAlreadyLocked)
	}
}

func TestAcquireAllowsIndependentLocks(t *testing.T) {
	baseDir := t.TempDir()

	first, err := Acquire(filepath.Join(baseDir, "first.lock"))
	if err != nil {
		t.Fatalf("Acquire() first error = %v", err)
	}
	t.Cleanup(func() {
		if err := first.Release(); err != nil {
			t.Fatalf("Release() first error = %v", err)
		}
	})

	second, err := Acquire(filepath.Join(baseDir, "second.lock"))
	if err != nil {
		t.Fatalf("Acquire() second error = %v", err)
	}
	t.Cleanup(func() {
		if err := second.Release(); err != nil {
			t.Fatalf("Release() second error = %v", err)
		}
	})
}

func TestReleaseAllowsReacquire(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "demo.lock")

	first, err := Acquire(lockPath)
	if err != nil {
		t.Fatalf("Acquire() first error = %v", err)
	}

	if err := first.Release(); err != nil {
		t.Fatalf("Release() error = %v", err)
	}

	second, err := Acquire(lockPath)
	if err != nil {
		t.Fatalf("Acquire() second error = %v", err)
	}
	t.Cleanup(func() {
		if err := second.Release(); err != nil {
			t.Fatalf("Release() second error = %v", err)
		}
	})
}
