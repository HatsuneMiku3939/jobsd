//go:build windows

package app

import (
	"os/exec"
	"testing"

	"golang.org/x/sys/windows"
)

func TestConfigureDetachedProcess(t *testing.T) {
	cmd := exec.Command("cmd.exe", "/C", "echo", "jobsd")

	configureDetachedProcess(cmd)

	if cmd.SysProcAttr == nil {
		t.Fatal("SysProcAttr = nil, want configured attributes")
	}
	if !cmd.SysProcAttr.HideWindow {
		t.Fatal("HideWindow = false, want true")
	}

	wantFlags := uint32(windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP)
	if cmd.SysProcAttr.CreationFlags != wantFlags {
		t.Fatalf("CreationFlags = %#x, want %#x", cmd.SysProcAttr.CreationFlags, wantFlags)
	}
}
