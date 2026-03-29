//go:build !windows

package daemon

func shellCommand(command string) (string, []string) {
	return "sh", []string{"-lc", command}
}
