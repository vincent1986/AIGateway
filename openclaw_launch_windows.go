//go:build windows

package main

import (
	"os/exec"
	"syscall"
)

func configureDetachedCmd(cmd *exec.Cmd) {
	const (
		detachedProcess       = 0x00000008
		createNewProcessGroup = 0x00000200
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: detachedProcess | createNewProcessGroup,
	}
}
