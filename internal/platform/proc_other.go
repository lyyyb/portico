//go:build !windows

package platform

import (
	"os/exec"
	"syscall"
)

// PrepareChild 让子进程独立成组，便于整组回收
func PrepareChild(cmd *exec.Cmd, _ string) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// ShellOpen 在非 Windows 平台直接启动进程
func ShellOpen(exe string, args []string, dir string) error {
	cmd := exec.Command(exe, args...)
	cmd.Dir = dir
	return cmd.Start()
}

// KillTree 结束进程组
func KillTree(pid int) {
	_ = syscall.Kill(-pid, syscall.SIGKILL)
}
