//go:build windows

package platform

import (
	"strings"
	"syscall"
	"unsafe"
)

// processEntry32 对应 Windows 的 PROCESSENTRY32W
type processEntry32 struct {
	size            uint32
	cntUsage        uint32
	processID       uint32
	defaultHeapID   uintptr
	moduleID        uint32
	cntThreads      uint32
	parentProcessID uint32
	priClassBase    int32
	flags           uint32
	exeFile         [syscall.MAX_PATH]uint16
}

const th32csSnapProcess = 0x00000002

var (
	procCreateToolhelp32Snapshot = kernel32.NewProc("CreateToolhelp32Snapshot")
	procProcess32FirstW          = kernel32.NewProc("Process32FirstW")
	procProcess32NextW           = kernel32.NewProc("Process32NextW")
)

// Processes 列出系统进程：小写镜像名 -> 进程号列表
func Processes() (map[string][]int, error) {
	snap, _, err := procCreateToolhelp32Snapshot.Call(th32csSnapProcess, 0)
	if snap == uintptr(syscall.InvalidHandle) {
		return nil, err
	}
	defer syscall.CloseHandle(syscall.Handle(snap))

	entry := processEntry32{size: uint32(unsafe.Sizeof(processEntry32{}))}
	ok, _, _ := procProcess32FirstW.Call(snap, uintptr(unsafe.Pointer(&entry)))
	if ok == 0 {
		return nil, nil
	}

	out := make(map[string][]int)
	for {
		if name := strings.ToLower(syscall.UTF16ToString(entry.exeFile[:])); name != "" {
			out[name] = append(out[name], int(entry.processID))
		}
		entry.size = uint32(unsafe.Sizeof(processEntry32{}))
		if ok, _, _ = procProcess32NextW.Call(snap, uintptr(unsafe.Pointer(&entry))); ok == 0 {
			break
		}
	}
	return out, nil
}
