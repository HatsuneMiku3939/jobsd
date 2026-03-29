//go:build !unix

package lock

import (
	"fmt"
	"os"
)

func tryLock(file *os.File) error {
	return fmt.Errorf("file locking is not implemented on this platform")
}

func unlock(file *os.File) error {
	return nil
}
