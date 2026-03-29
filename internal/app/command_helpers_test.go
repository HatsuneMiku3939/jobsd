package app

import (
	"bytes"
	"context"
	"testing"
	"time"
)

func executeRootCommand(t *testing.T, args ...string) (string, error) {
	t.Helper()

	stdout := &bytes.Buffer{}
	cmd := NewRootCommand(BuildInfo{Version: "v1.0.0"}, stdout, &bytes.Buffer{})
	cmd.SetArgs(args)

	err := cmd.Execute()
	return stdout.String(), err
}

func setFixedCurrentTime(t *testing.T, value time.Time) {
	t.Helper()

	original := currentTime
	currentTime = func() time.Time {
		return value.UTC()
	}

	t.Cleanup(func() {
		currentTime = original
	})
}

func openInstanceDBForTest(t *testing.T, instance string) (*instanceDB, func()) {
	t.Helper()

	db, cleanup, err := openInstanceDB(context.Background(), instance)
	if err != nil {
		t.Fatalf("openInstanceDB() error = %v", err)
	}

	return db, func() {
		if err := cleanup(); err != nil {
			t.Fatalf("cleanup() error = %v", err)
		}
	}
}
