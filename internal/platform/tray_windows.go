//go:build windows

package platform

import (
	"log"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	procRegisterClassEx     = user32.NewProc("RegisterClassExW")
	procCreateWindowEx      = user32.NewProc("CreateWindowExW")
	procDefWindowProc       = user32.NewProc("DefWindowProcW")
	procGetMessage          = user32.NewProc("GetMessageW")
	procTranslateMessage    = user32.NewProc("TranslateMessage")
	procDispatchMessage     = user32.NewProc("DispatchMessageW")
	procPostQuitMessage     = user32.NewProc("PostQuitMessage")
	procDestroyWindow       = user32.NewProc("DestroyWindow")
	procCreatePopupMenu     = user32.NewProc("CreatePopupMenu")
	procAppendMenu          = user32.NewProc("AppendMenuW")
	procTrackPopupMenu      = user32.NewProc("TrackPopupMenu")
	procDestroyMenu         = user32.NewProc("DestroyMenu")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
	procGetCursorPos        = user32.NewProc("GetCursorPos")
	procLoadImage           = user32.NewProc("LoadImageW")
	procLoadIcon            = user32.NewProc("LoadIconW")
	procMessageBox          = user32.NewProc("MessageBoxW")
	procShellNotifyIcon     = shell32.NewProc("Shell_NotifyIconW")
	procGetModuleHandle     = kernel32.NewProc("GetModuleHandleW")
)

const (
	wmDestroy       = 0x0002
	wmCommand       = 0x0111
	wmLButtonUp     = 0x0202
	wmLButtonDblClk = 0x0203
	wmRButtonUp     = 0x0205
	trayMsgID       = 0x8000 + 1 // WM_APP + 1

	nimAdd    = 0x00000000
	nimDelete = 0x00000002

	nifMessage = 0x00000001
	nifIcon    = 0x00000002
	nifTip     = 0x00000004

	idiApplication = 32512
	trayIconID     = 1
	imageIcon      = 1
	lrLoadFromFile = 0x00000010
	lrDefaultSize  = 0x00000040

	mfString       = 0x00000000
	mfGrayed       = 0x00000001
	mfPopup        = 0x00000010
	mfSeparator    = 0x00000800
	tpmRightButton = 0x0002
	tpmBottomAlign = 0x0020

	mbOK       = 0x00000000
	mbIconInfo = 0x00000040

	cmdOpen    = 1001
	cmdQuit    = 1002
	cmdStopAll = 1003
	cmdOpenLog = 1004
	cmdAbout   = 1005
	cmdAppBase = 2000 // 每个运行中的应用占两个命令 ID：打开、停止

	hwndMessage = ^uintptr(2) // (HWND)-3，仅接收消息的窗口
)

type wndClassEx struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     syscall.Handle
	hIcon         syscall.Handle
	hCursor       syscall.Handle
	hbrBackground syscall.Handle
	lpszMenuName  *uint16
	lpszClassName *uint16
	hIconSm       syscall.Handle
}

type notifyIconData struct {
	cbSize           uint32
	hWnd             syscall.Handle
	uID              uint32
	uFlags           uint32
	uCallbackMessage uint32
	hIcon            syscall.Handle
	szTip            [128]uint16
	dwState          uint32
	dwStateMask      uint32
	szInfo           [256]uint16
	uTimeoutVersion  uint32
	szInfoTitle      [64]uint16
	dwInfoFlags      uint32
	guidItem         [16]byte
	hBalloonIcon     syscall.Handle
}

type point struct {
	x int32
	y int32
}

type windowMsg struct {
	hwnd     syscall.Handle
	message  uint32
	wParam   uintptr
	lParam   uintptr
	time     uint32
	pt       point
	lPrivate uint32
}

type tray struct {
	hwnd     syscall.Handle
	handlers TrayHandlers

	menuApps []TrayApp // 本次菜单展开时运行中的应用，用于解析子菜单命令
}

var activeTray *tray

// RunTray 创建托盘图标并进入消息循环，阻塞直到用户选择退出
func RunTray(h TrayHandlers) {
	t := &tray{handlers: h}
	if err := t.start(); err != nil {
		// 托盘不可用时不影响主功能，继续托管页面
		log.Printf("托盘启动失败: %v", err)
		select {}
	}
	t.loop()
}

func (t *tray) start() error {
	hInst, _, _ := procGetModuleHandle.Call(0)
	className, err := syscall.UTF16PtrFromString("PorticoTrayWindow")
	if err != nil {
		return err
	}

	wc := wndClassEx{
		lpfnWndProc:   syscall.NewCallback(wndProc),
		hInstance:     syscall.Handle(hInst),
		lpszClassName: className,
	}
	wc.cbSize = uint32(unsafe.Sizeof(wc))
	if r, _, e := procRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
		return e
	}

	hwnd, _, e := procCreateWindowEx.Call(
		0, uintptr(unsafe.Pointer(className)), 0, 0,
		0, 0, 0, 0, hwndMessage, 0, hInst, 0,
	)
	if hwnd == 0 {
		return e
	}

	t.hwnd = syscall.Handle(hwnd)
	activeTray = t
	return t.addIcon()
}

func (t *tray) addIcon() error {
	nid := notifyIconData{
		hWnd:             t.hwnd,
		uID:              1,
		uFlags:           nifMessage | nifIcon | nifTip,
		uCallbackMessage: trayMsgID,
		hIcon:            loadTrayIcon(),
	}
	nid.cbSize = uint32(unsafe.Sizeof(nid))
	copy(nid.szTip[:], syscall.StringToUTF16("Portico"))

	if r, _, e := procShellNotifyIcon.Call(nimAdd, uintptr(unsafe.Pointer(&nid))); r == 0 {
		return e
	}
	return nil
}

func (t *tray) removeIcon() {
	nid := notifyIconData{hWnd: t.hwnd, uID: 1}
	nid.cbSize = uint32(unsafe.Sizeof(nid))
	procShellNotifyIcon.Call(nimDelete, uintptr(unsafe.Pointer(&nid)))
}

func appendItem(menu uintptr, flags, id uintptr, text string) {
	p, _ := syscall.UTF16PtrFromString(text)
	procAppendMenu.Call(menu, flags, id, uintptr(unsafe.Pointer(p)))
}

func (t *tray) showMenu() {
	menu, _, _ := procCreatePopupMenu.Call()
	if menu == 0 {
		return
	}
	defer procDestroyMenu.Call(menu)

	appendItem(menu, mfString, cmdOpen, "打开 Portico")
	appendItem(menu, mfString, cmdOpenLog, "打开日志")
	appendItem(menu, mfSeparator, 0, "")

	// 运行中的应用用二级菜单，每个可单独打开或停止
	var apps []TrayApp
	if t.handlers.Running != nil {
		apps = t.handlers.Running()
	}
	t.menuApps = apps
	if len(apps) == 0 {
		appendItem(menu, mfString|mfGrayed, 0, "暂无运行中的应用")
	} else {
		for i, app := range apps {
			sub, _, _ := procCreatePopupMenu.Call()
			if sub == 0 {
				continue
			}
			appendItem(sub, mfString, uintptr(cmdAppBase+i*2), "打开页面")
			appendItem(sub, mfString, uintptr(cmdAppBase+i*2+1), "停止")
			appendItem(menu, mfPopup, sub, app.Name)
		}
	}

	appendItem(menu, mfSeparator, 0, "")
	appendItem(menu, mfString, cmdStopAll, "停止全部应用")
	appendItem(menu, mfString, cmdAbout, "关于")
	appendItem(menu, mfSeparator, 0, "")
	appendItem(menu, mfString, cmdQuit, "退出")

	var pt point
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	procSetForegroundWindow.Call(uintptr(t.hwnd))
	procTrackPopupMenu.Call(
		menu, tpmRightButton|tpmBottomAlign,
		uintptr(uint32(pt.x)), uintptr(uint32(pt.y)), 0, uintptr(t.hwnd), 0,
	)
}

func (t *tray) loop() {
	var m windowMsg
	for {
		r, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			return
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessage.Call(uintptr(unsafe.Pointer(&m)))
	}
}

func wndProc(hwnd syscall.Handle, msg uint32, wParam, lParam uintptr) uintptr {
	t := activeTray
	switch msg {
	case trayMsgID:
		if t == nil {
			return 0
		}
		switch lParam {
		case wmLButtonUp, wmLButtonDblClk:
			go t.handlers.Open()
		case wmRButtonUp:
			t.showMenu()
		}
		return 0
	case wmCommand:
		if t == nil {
			return 0
		}
		id := uint32(wParam & 0xffff)
		switch {
		case id == cmdOpen:
			go t.handlers.Open()
		case id == cmdStopAll:
			if t.handlers.StopAll != nil {
				go t.handlers.StopAll()
			}
		case id == cmdOpenLog:
			if t.handlers.OpenLog != nil {
				go t.handlers.OpenLog()
			}
		case id == cmdAbout:
			if t.handlers.About != nil {
				go t.handlers.About()
			}
		case id == cmdQuit:
			if t.handlers.StopAll != nil {
				// 同步执行，保证子进程随退出一起被回收
				t.handlers.StopAll()
			}
			t.removeIcon()
			procDestroyWindow.Call(uintptr(hwnd))
		case id >= cmdAppBase:
			idx := int((id - cmdAppBase) / 2)
			if idx < len(t.menuApps) {
				app := t.menuApps[idx]
				if (id-cmdAppBase)%2 == 0 {
					if t.handlers.OpenApp != nil {
						go t.handlers.OpenApp(app.Key)
					}
				} else if t.handlers.StopApp != nil {
					go t.handlers.StopApp(app.Key)
				}
			}
		}
		return 0
	case wmDestroy:
		procPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := procDefWindowProc.Call(uintptr(hwnd), uintptr(msg), wParam, lParam)
	return r
}

// Notice 弹出系统提示框
func Notice(title, text string) {
	t, _ := syscall.UTF16PtrFromString(title)
	m, _ := syscall.UTF16PtrFromString(text)
	procMessageBox.Call(0, uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(t)), mbOK|mbIconInfo)
}

// loadTrayIcon 优先读 exe 自身编入的图标资源（cmd/portico/rsrc_windows_*.syso），
// 其次兼容旧部署的同目录 icon.ico，都没有时用系统默认图标
func loadTrayIcon() syscall.Handle {
	if hInst, _, _ := procGetModuleHandle.Call(0); hInst != 0 {
		if h, _, _ := procLoadImage.Call(hInst, trayIconID, imageIcon, 0, 0, lrDefaultSize); h != 0 {
			return syscall.Handle(h)
		}
	}
	if exe, err := os.Executable(); err == nil {
		p, err := syscall.UTF16PtrFromString(filepath.Join(filepath.Dir(exe), "icon.ico"))
		if err == nil {
			if h, _, _ := procLoadImage.Call(
				0, uintptr(unsafe.Pointer(p)), imageIcon, 0, 0,
				lrLoadFromFile|lrDefaultSize,
			); h != 0 {
				return syscall.Handle(h)
			}
		}
	}
	h, _, _ := procLoadIcon.Call(0, uintptr(idiApplication))
	return syscall.Handle(h)
}
