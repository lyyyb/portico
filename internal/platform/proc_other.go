//go:build !windows

package platform

import (
	"os/exec"
	"syscall"
)

// PrepareChild 让子进程独立成组，便于整组回收
func PrepareChild(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// KillTree 结束进程组
func KillTree(pid int) {
	_ = syscall.Kill(-pid, syscall.SIGKILL)
}
