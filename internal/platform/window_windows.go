//go:build windows

package platform

import (
	"runtime"
	"syscall"
	"unsafe"
)

var (
	procEnumWindows              = user32.NewProc("EnumWindows")
	procIsWindowVisible          = user32.NewProc("IsWindowVisible")
	procGetWindowThreadProcessID = user32.NewProc("GetWindowThreadProcessId")
	procGetWindowTextLength      = user32.NewProc("GetWindowTextLengthW")
	procGetWindow                = user32.NewProc("GetWindow")
	procIsIconic                 = user32.NewProc("IsIconic")
	procShowWindow               = user32.NewProc("ShowWindow")
	procBringWindowToTop         = user32.NewProc("BringWindowToTop")
	procGetForegroundWindow      = user32.NewProc("GetForegroundWindow")
	procAttachThreadInput        = user32.NewProc("AttachThreadInput")
	procGetCurrentThreadID       = kernel32.NewProc("GetCurrentThreadId")
)

const (
	swRestore = 9
	swShow    = 5
	gwOwner   = 4
)

// ActivateWindows 找到属于这些进程的可见顶层窗口，恢复并切到前台；
// 一个窗口都没找到时返回 false，供调用方回退到打开网页
func ActivateWindows(pids []int) bool {
	if len(pids) == 0 {
		return false
	}
	want := make(map[uint32]bool, len(pids))
	for _, pid := range pids {
		want[uint32(pid)] = true
	}

	// 优先找“可见、有标题、无 owner”的主窗口；找不到再退而取该进程任意可见窗口
	var target, fallback syscall.Handle
	cb := syscall.NewCallback(func(hwnd syscall.Handle, _ uintptr) uintptr {
		if target != 0 {
			return 0
		}
		if v, _, _ := procIsWindowVisible.Call(uintptr(hwnd)); v == 0 {
			return 1
		}
		var pid uint32
		procGetWindowThreadProcessID.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&pid)))
		if !want[pid] {
			return 1
		}
		owner, _, _ := procGetWindow.Call(uintptr(hwnd), gwOwner)
		titled, _, _ := procGetWindowTextLength.Call(uintptr(hwnd))
		if owner == 0 && titled != 0 {
			target = hwnd
			return 0
		}
		if fallback == 0 {
			fallback = hwnd
		}
		return 1
	})
	procEnumWindows.Call(cb, 0)

	if target == 0 {
		target = fallback
	}
	if target == 0 {
		return false
	}

	// 前台焦点属于别的进程时，系统会忽略 SetForegroundWindow，
	// 先把自己的输入队列挂到当前前台线程上再设置，与常见托盘程序做法一致
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if v, _, _ := procIsIconic.Call(uintptr(target)); v != 0 {
		procShowWindow.Call(uintptr(target), swRestore)
	} else {
		procShowWindow.Call(uintptr(target), swShow)
	}
	procBringWindowToTop.Call(uintptr(target))

	fg, _, _ := procGetForegroundWindow.Call()
	fgThread, _, _ := procGetWindowThreadProcessID.Call(fg, 0)
	curThread, _, _ := procGetCurrentThreadID.Call()
	if fgThread != 0 && fgThread != curThread {
		procAttachThreadInput.Call(curThread, fgThread, 1)
		procSetForegroundWindow.Call(uintptr(target))
		procAttachThreadInput.Call(curThread, fgThread, 0)
	} else {
		procSetForegroundWindow.Call(uintptr(target))
	}
	return true
}
