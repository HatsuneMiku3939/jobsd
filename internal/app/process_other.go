//go:build !unix && !windows

package app

import "os/exec"

func configureDetachedProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = nil
}
