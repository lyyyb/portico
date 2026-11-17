//go:build !windows

package platform

import (
	"log"
	"os"
	"os/signal"
)

// RunTray 在非 Windows 平台没有托盘，阻塞到进程被中断
func RunTray(_ TrayHandlers) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt)
	<-ch
}

// Notice 在非 Windows 平台写入日志
func Notice(title, text string) {
	log.Printf("%s: %s", title, text)
}
