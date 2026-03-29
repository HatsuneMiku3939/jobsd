//go:build !windows

package daemon

func shouldRetryStateRemove(err error) bool {
	return false
}
