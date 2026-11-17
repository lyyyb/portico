//go:build !windows

package platform

import (
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// Processes 列出系统进程：小写镜像名 -> 进程号列表
func Processes() (map[string][]int, error) {
	out, err := exec.Command("ps", "-eo", "pid=,comm=").Output()
	if err != nil {
		return nil, err
	}

	all := make(map[string][]int)
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		pid, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		name := strings.ToLower(filepath.Base(fields[1]))
		all[name] = append(all[name], pid)
	}
	return all, nil
}
