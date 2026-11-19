//go:build windows

package platform

import (
	"syscall"
	"unsafe"
)

// ShowError 弹出系统错误提示。启动器是 windowsgui 程序，没有控制台，
// 启动阶段的致命错误必须用对话框告知用户。
func ShowError(title, msg string) {
	t, _ := syscall.UTF16PtrFromString(title)
	m, _ := syscall.UTF16PtrFromString(msg)
	procMessageBox.Call(0, uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(t)), mbOK|mbIconErr)
}
