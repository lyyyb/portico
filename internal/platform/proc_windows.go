//go:build windows

package platform

import (
	"os/exec"
	"strconv"
	"syscall"
)

// PrepareChild 让子进程不弹出控制台窗口
func PrepareChild(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
}

// KillTree 结束进程及其全部子进程
func KillTree(pid int) {
	_ = exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/T", "/F").Run()
}
