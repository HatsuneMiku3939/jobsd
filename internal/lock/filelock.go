package lock

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

var ErrAlreadyLocked = errors.New("file lock is already held")

type FileLock struct {
	file *os.File
	path string
}

func Acquire(path string) (*FileLock, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create lock directory: %w", err)
	}

	file, err := openLockedFile(path)
	if err != nil {
		return nil, err
	}

	return &FileLock{
		file: file,
		path: path,
	}, nil
}

func (l *FileLock) Release() error {
	if l == nil || l.file == nil {
		return nil
	}

	if err := l.file.Close(); err != nil {
		l.file = nil
		return fmt.Errorf("close %s: %w", l.path, err)
	}

	l.file = nil
	return nil
}
