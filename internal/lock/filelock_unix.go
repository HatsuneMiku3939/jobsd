//go:build unix

package lock

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

func tryLock(file *os.File) error {
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return ErrAlreadyLocked
		}
		return fmt.Errorf("acquire file lock: %w", err)
	}

	return nil
}

func unlock(file *os.File) error {
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_UN); err != nil {
		return fmt.Errorf("release file lock: %w", err)
	}

	return nil
}
