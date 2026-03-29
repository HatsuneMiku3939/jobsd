//go:build windows

package lock

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestAcquireWindowsMapsExclusiveOpenErrors(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "jobsd.lock")

	first, err := Acquire(lockPath)
	if err != nil {
		t.Fatalf("Acquire() first error = %v", err)
	}
	defer func() {
		if err := first.Release(); err != nil {
			t.Fatalf("Release() first error = %v", err)
		}
	}()

	second, err := Acquire(lockPath)
	if second != nil {
		t.Fatal("Acquire() second lock = non-nil, want nil")
	}
	if !errors.Is(err, ErrAlreadyLocked) {
		t.Fatalf("Acquire() second error = %v, want %v", err, ErrAlreadyLocked)
	}
}
