//go:build windows

package platform

import (
	"os/exec"
	"strconv"
	"syscall"
)

// createNoWindow 避免辅助命令（taskkill）弹出控制台窗口
const createNoWindow = 0x08000000

// KillTree 结束进程及其全部子进程
func KillTree(pid int) {
	cmd := exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/T", "/F")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow,
	}
	_ = cmd.Run()
}
