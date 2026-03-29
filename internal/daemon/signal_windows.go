//go:build windows

package daemon

import "os"

func shutdownSignals() []os.Signal {
	return []os.Signal{os.Interrupt}
}
