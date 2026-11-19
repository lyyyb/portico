package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"

	"portico/internal/launcher"
	"portico/internal/platform"
	"portico/internal/project"
	"portico/internal/version"
)

func main() {
	logPath := setupLog()

	store, err := project.Open()
	if err != nil {
		log.Fatalf("创建 apps 目录失败: %v", err)
	}

	projects, err := store.Scan()
	if err != nil {
		// 配置文件有误时进程会直接退出，GUI 程序没有控制台，必须弹窗告知
		log.Printf("读取配置失败: %v", err)
		platform.ShowError("Portico 配置有误", fmt.Sprintf("%v\n\n请修正启动器目录下的 %s 后重新启动 Portico。", err, project.ConfigFile))
		return
	}
	if len(projects) == 0 {
		log.Printf("未发现应用，请把前端打包产物放入 %s", store.Root())
	}

	ln, err := net.Listen("tcp", launcher.Addr)
	if err != nil {
		if launcher.Alive() {
			// 自己已在运行，直接把界面叫出来
			platform.OpenBrowser(launcher.URL())
			return
		}
		log.Fatalf("端口 %s 被其他程序占用，无法启动", launcher.Addr)
	}

	srv := launcher.New(store, projects)
	log.Printf("启动器: %s（发现 %d 个应用，目录 %s）", launcher.URL(), len(projects), store.Root())
	go func() {
		if err := http.Serve(ln, srv.Routes()); err != nil {
			log.Printf("启动器已停止: %v", err)
		}
	}()

	platform.OpenBrowser(launcher.URL())
	platform.RunTray(platform.TrayHandlers{
		Open:    func() { platform.OpenBrowser(launcher.URL()) },
		Running: srv.RunningApps,
		OpenApp: srv.OpenApp,
		StopApp: srv.StopApp,
		StopAll: srv.StopAll,
		OpenLog: func() { platform.RevealFile(logPath) },
		About: func() {
			platform.Notice("关于 Portico", fmt.Sprintf(
				"Portico v%s\n\n应用目录：%s\n日志文件：%s",
				version.Version, store.Root(), logPath,
			))
		},
	})
	os.Exit(0)
}

// setupLog 把日志同时写到 app.log，避免 windowsgui 模式下错误信息无处可看
func setupLog() string {
	logPath := filepath.Join(platform.BaseDir(), "app.log")

	// 超过 1 MB 时轮换为 app.log.1，避免长期运行无限增长
	const maxLogSize = 1 << 20
	if info, err := os.Stat(logPath); err == nil && info.Size() > maxLogSize {
		backup := logPath + ".1"
		_ = os.Remove(backup)
		_ = os.Rename(logPath, backup)
	}

	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return logPath
	}
	// 文件放前面：windowsgui 模式下 stderr 不可写，MultiWriter 遇错会中断后续写入
	log.SetOutput(io.MultiWriter(f, os.Stderr))
	return logPath
}
