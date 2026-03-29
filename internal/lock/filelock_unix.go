//go:build unix

package lock

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

func openLockedFile(path string) (*os.File, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open lock file: %w", err)
	}

	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, ErrAlreadyLocked
		}
		return nil, fmt.Errorf("acquire file lock: %w", err)
	}

	return file, nil
}
