package platform

import (
	"path/filepath"
	"strings"
	"time"
)

// WaitProcess 在超时时间内等待同名进程出现，用于确认系统确实启动了程序
func WaitProcess(image string, timeout time.Duration) bool {
	if len(ProcessPIDs(image)) > 0 {
		return true
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
		if len(ProcessPIDs(image)) > 0 {
			return true
		}
	}
	return false
}

// ProcessPIDs 按可执行文件名（可传完整路径）查找正在运行的进程号
func ProcessPIDs(image string) []int {
	all, err := Processes()
	if err != nil {
		return nil
	}
	return all[strings.ToLower(filepath.Base(image))]
}
