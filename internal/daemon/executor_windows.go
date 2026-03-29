//go:build windows

package daemon

func shellCommand(command string) (string, []string) {
	return "cmd", []string{"/C", command}
}
