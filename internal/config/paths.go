package config

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	dataDirNamespace    = "jobsd"
	runtimeDirNamespace = "jobsd"
	databaseFileName    = "jobs.db"
	stateFileName       = "state.json"
)

var instanceNamePattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

type Paths struct {
	Instance     string
	DataDir      string
	RuntimeDir   string
	DatabasePath string
	LockPath     string
	StatePath    string
}

func ResolvePaths(instance string) (Paths, error) {
	if err := validateInstanceName(instance); err != nil {
		return Paths{}, err
	}

	dataBaseDir, err := resolveDataBaseDir()
	if err != nil {
		return Paths{}, err
	}

	runtimeBaseDir := resolveRuntimeBaseDir()
	dataDir := filepath.Join(dataBaseDir, "instances", instance)
	runtimeDir := filepath.Join(runtimeBaseDir, instance)

	return Paths{
		Instance:     instance,
		DataDir:      dataDir,
		RuntimeDir:   runtimeDir,
		DatabasePath: filepath.Join(dataDir, databaseFileName),
		LockPath:     filepath.Join(runtimeBaseDir, instance+".lock"),
		StatePath:    filepath.Join(runtimeDir, stateFileName),
	}, nil
}

func validateInstanceName(instance string) error {
	if instance == "" {
		return fmt.Errorf("instance name is required")
	}
	if strings.ContainsAny(instance, `/\`) {
		return fmt.Errorf("instance name %q must not contain path separators", instance)
	}
	if !instanceNamePattern.MatchString(instance) {
		return fmt.Errorf("instance name %q must match %s", instance, instanceNamePattern.String())
	}
	return nil
}

func resolveDataBaseDir() (string, error) {
	if baseDir := os.Getenv("XDG_DATA_HOME"); baseDir != "" {
		return filepath.Join(baseDir, dataDirNamespace), nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home directory: %w", err)
	}

	return filepath.Join(homeDir, ".local", "share", dataDirNamespace), nil
}

func resolveRuntimeBaseDir() string {
	if baseDir := os.Getenv("XDG_RUNTIME_DIR"); baseDir != "" {
		return filepath.Join(baseDir, runtimeDirNamespace)
	}

	return filepath.Join(os.TempDir(), "jobsd-"+currentUserID())
}

func currentUserID() string {
	currentUser, err := user.Current()
	if err != nil || currentUser.Uid == "" {
		return "0"
	}

	return currentUser.Uid
}
