package daemon

import (
	"os"
	"testing"
)

func osExecutablePath(t *testing.T) string {
	t.Helper()

	path, err := os.Executable()
	if err != nil {
		t.Fatalf("Executable() error = %v", err)
	}

	return path
}
