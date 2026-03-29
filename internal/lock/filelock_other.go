//go:build !unix && !windows

package lock

import (
	"fmt"
	"os"
)

func openLockedFile(path string) (*os.File, error) {
	return nil, fmt.Errorf("file locking is not implemented on this platform")
}
