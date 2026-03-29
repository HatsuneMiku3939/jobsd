//go:build windows

package lock

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

func openLockedFile(path string) (*os.File, error) {
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, fmt.Errorf("encode lock file path: %w", err)
	}

	handle, err := windows.CreateFile(
		pathPtr,
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		0,
		nil,
		windows.OPEN_ALWAYS,
		windows.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	if err != nil {
		if errors.Is(err, windows.ERROR_SHARING_VIOLATION) || errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
			return nil, ErrAlreadyLocked
		}
		return nil, fmt.Errorf("open lock file: %w", err)
	}

	return os.NewFile(uintptr(handle), path), nil
}
