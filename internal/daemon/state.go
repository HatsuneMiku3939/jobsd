package daemon

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/hatsunemiku3939/jobsd/internal/domain"
)

var (
	ErrStateNotFound = errors.New("scheduler state file not found")
	ErrStateCorrupt  = errors.New("scheduler state file is corrupt")
)

type schedulerStateJSON struct {
	Instance  string `json:"instance"`
	PID       int    `json:"pid"`
	Port      int    `json:"port"`
	Token     string `json:"token"`
	DBPath    string `json:"db_path"`
	StartedAt string `json:"started_at"`
	Version   string `json:"version"`
}

func WriteState(path string, state domain.SchedulerState) error {
	payload, err := marshalSchedulerState(state)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create state directory: %w", err)
	}

	tempFile, err := os.CreateTemp(filepath.Dir(path), "state-*.json")
	if err != nil {
		return fmt.Errorf("create temporary state file: %w", err)
	}
	tempPath := tempFile.Name()
	defer func() {
		_ = os.Remove(tempPath)
	}()

	if err := tempFile.Chmod(0o600); err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("chmod temporary state file: %w", err)
	}

	if _, err := tempFile.Write(payload); err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("write temporary state file: %w", err)
	}

	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("close temporary state file: %w", err)
	}

	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("replace state file: %w", err)
	}

	return nil
}

func ReadState(path string) (domain.SchedulerState, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return domain.SchedulerState{}, ErrStateNotFound
		}
		return domain.SchedulerState{}, fmt.Errorf("read state file: %w", err)
	}

	var raw schedulerStateJSON
	if err := json.Unmarshal(payload, &raw); err != nil {
		return domain.SchedulerState{}, fmt.Errorf("%w: decode json: %v", ErrStateCorrupt, err)
	}

	return decodeSchedulerState(raw)
}

func RemoveState(path string) error {
	if err := os.Remove(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("remove state file: %w", err)
	}

	return nil
}

func marshalSchedulerState(state domain.SchedulerState) ([]byte, error) {
	if err := validateSchedulerState(state); err != nil {
		return nil, err
	}

	raw := schedulerStateJSON{
		Instance:  state.Instance,
		PID:       state.PID,
		Port:      state.Port,
		Token:     state.Token,
		DBPath:    state.DBPath,
		StartedAt: state.StartedAt.UTC().Format(time.RFC3339),
		Version:   state.Version,
	}

	payload, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode state json: %w", err)
	}

	return append(payload, '\n'), nil
}

func decodeSchedulerState(raw schedulerStateJSON) (domain.SchedulerState, error) {
	if raw.Instance == "" {
		return domain.SchedulerState{}, fmt.Errorf("%w: instance is required", ErrStateCorrupt)
	}
	if raw.PID <= 0 {
		return domain.SchedulerState{}, fmt.Errorf("%w: pid must be positive", ErrStateCorrupt)
	}
	if raw.Port <= 0 {
		return domain.SchedulerState{}, fmt.Errorf("%w: port must be positive", ErrStateCorrupt)
	}
	if raw.Token == "" {
		return domain.SchedulerState{}, fmt.Errorf("%w: token is required", ErrStateCorrupt)
	}
	if raw.DBPath == "" {
		return domain.SchedulerState{}, fmt.Errorf("%w: db_path is required", ErrStateCorrupt)
	}
	if raw.Version == "" {
		return domain.SchedulerState{}, fmt.Errorf("%w: version is required", ErrStateCorrupt)
	}
	if raw.StartedAt == "" {
		return domain.SchedulerState{}, fmt.Errorf("%w: started_at is required", ErrStateCorrupt)
	}

	startedAt, err := time.Parse(time.RFC3339, raw.StartedAt)
	if err != nil {
		return domain.SchedulerState{}, fmt.Errorf("%w: started_at: %v", ErrStateCorrupt, err)
	}

	return domain.SchedulerState{
		Instance:  raw.Instance,
		PID:       raw.PID,
		Port:      raw.Port,
		Token:     raw.Token,
		DBPath:    raw.DBPath,
		StartedAt: startedAt.UTC(),
		Version:   raw.Version,
	}, nil
}

func validateSchedulerState(state domain.SchedulerState) error {
	if state.Instance == "" {
		return fmt.Errorf("state instance is required")
	}
	if state.PID <= 0 {
		return fmt.Errorf("state pid must be positive")
	}
	if state.Port <= 0 {
		return fmt.Errorf("state port must be positive")
	}
	if state.Token == "" {
		return fmt.Errorf("state token is required")
	}
	if state.DBPath == "" {
		return fmt.Errorf("state db path is required")
	}
	if state.Version == "" {
		return fmt.Errorf("state version is required")
	}
	if state.StartedAt.IsZero() {
		return fmt.Errorf("state started at is required")
	}

	return nil
}
