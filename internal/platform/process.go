package platform

import (
	"path/filepath"
	"strings"
)

// ProcessPIDs 按可执行文件名（可传完整路径）查找正在运行的进程号
func ProcessPIDs(image string) []int {
	all, err := Processes()
	if err != nil {
		return nil
	}
	return all[strings.ToLower(filepath.Base(image))]
}
