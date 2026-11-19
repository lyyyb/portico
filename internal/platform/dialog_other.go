//go:build !windows

package platform

// ShowError 在非 Windows 平台没有等价对话框，保持空实现
func ShowError(title, msg string) {}
