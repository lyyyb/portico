//go:build windows

package platform

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"unsafe"
)

const (
	createNewConsole = 0x00000010

	subsystemGUI = 2 // IMAGE_SUBSYSTEM_WINDOWS_GUI
	subsystemCUI = 3 // IMAGE_SUBSYSTEM_WINDOWS_CUI

	swShowNormal = 1
)

var procShellExecuteW = shell32.NewProc("ShellExecuteW")

// PrepareChild 按可执行文件的子系统设置窗口行为：
// 控制台程序分配独立控制台（与双击一致）；图形程序不加任何窗口标志，
// 因为 HideWindow 会让系统用 SW_HIDE 启动，程序的首个窗口会一直隐藏
func PrepareChild(cmd *exec.Cmd, exe string) {
	if peSubsystem(exe) == subsystemCUI {
		cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNewConsole}
	}
}

// peSubsystem 读取 PE 可选头里的 Subsystem 字段，解析失败返回 0
func peSubsystem(path string) uint16 {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()

	buf := make([]byte, 4096)
	n, _ := f.Read(buf)
	buf = buf[:n]
	if len(buf) < 0x40 {
		return 0
	}

	lfanew := int(binary.LittleEndian.Uint32(buf[0x3c:0x40]))
	if lfanew+24+70 > len(buf) {
		return 0
	}
	if string(buf[lfanew:lfanew+4]) != "PE\x00\x00" {
		return 0
	}

	// Subsystem 位于可选头偏移 68 处，PE32 与 PE32+ 一致
	opt := lfanew + 24
	return binary.LittleEndian.Uint16(buf[opt+68 : opt+70])
}

// NeedsElevation 判断启动失败是否因为目标程序要求管理员权限
func NeedsElevation(err error) bool {
	return errors.Is(err, syscall.Errno(740)) // ERROR_ELEVATION_REQUIRED
}

// ShellOpen 交给系统按“双击”的方式打开程序：
// 控制台程序会自动获得控制台窗口，带管理员清单的程序会自动弹 UAC
func ShellOpen(exe string, args []string, dir string) error {
	return shellExec("open", exe, args, dir)
}

// RunElevated 通过系统 UAC 以管理员身份启动程序
func RunElevated(exe string, args []string, dir string) error {
	if err := shellExec("runas", exe, args, dir); err != nil {
		return fmt.Errorf("管理员授权被取消或启动失败: %w", err)
	}
	return nil
}

func shellExec(verb, exe string, args []string, dir string) error {
	v, _ := syscall.UTF16PtrFromString(verb)
	file, _ := syscall.UTF16PtrFromString(exe)
	params, _ := syscall.UTF16PtrFromString(escapeArgs(args))
	cwd, _ := syscall.UTF16PtrFromString(dir)

	r, _, _ := procShellExecuteW.Call(
		0,
		uintptr(unsafe.Pointer(v)),
		uintptr(unsafe.Pointer(file)),
		uintptr(unsafe.Pointer(params)),
		uintptr(unsafe.Pointer(cwd)),
		swShowNormal,
	)
	if r <= 32 {
		switch r {
		case 2, 3:
			return fmt.Errorf("找不到程序或工作目录")
		case 5:
			return fmt.Errorf("系统拒绝启动（权限不足或授权被取消）")
		case 31:
			return fmt.Errorf("没有关联的打开方式")
		}
		return fmt.Errorf("系统返回错误码 %d", r)
	}
	return nil
}

func escapeArgs(args []string) string {
	parts := make([]string, len(args))
	for i, a := range args {
		parts[i] = syscall.EscapeArg(a)
	}
	return strings.Join(parts, " ")
}
