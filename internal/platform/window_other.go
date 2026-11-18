//go:build !windows

package platform

// ActivateWindows 在非 Windows 平台不提供窗口激活，调用方会回退到打开网页
func ActivateWindows(pids []int) bool {
	return false
}
