//go:build unix

package app

import "syscall"

func newDetachedSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}
