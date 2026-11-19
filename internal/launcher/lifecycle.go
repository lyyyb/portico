package launcher

import (
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"portico/internal/platform"
	"portico/internal/project"
)

// startupGrace 是程序启动的观察期：熬过这段时间没退出，才认为启动成功
const startupGrace = 1500 * time.Millisecond

// procState 是一个程序型项目的运行态
type procState struct {
	cmd  *exec.Cmd
	done chan struct{}
}

func (s *Server) startStatic(in *instance) error {
	ln, err := reservePort(0)
	if err != nil {
		return err
	}
	port := ln.Addr().(*net.TCPAddr).Port
	url := "http://" + ln.Addr().String() + "/"
	srv := &http.Server{Handler: appHandler(in.p.Dir)}

	s.mu.Lock()
	in.srv, in.port, in.url = srv, port, url
	s.mu.Unlock()

	go func() {
		log.Printf("应用 %s 已启动: %s", in.p.Name, url)
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("应用 %s 已停止: %v", in.p.Name, err)
		}
	}()
	return nil
}

// startProgram 以系统默认方式打开程序（等同资源管理器里双击），
// 控制台、UAC、窗口显示都交给系统决定
func (s *Server) startProgram(in *instance) error {
	p := in.p
	// ShellExecute 无法注入环境变量，配置了 env 的程序退回直接创建进程
	if len(p.Env) > 0 {
		return s.startProgramExec(in)
	}
	if err := platform.ShellOpen(p.Exe, p.Args, filepath.Dir(p.Exe)); err != nil {
		return fmt.Errorf("无法启动: %w", err)
	}
	log.Printf("应用 %s 已通过系统打开: %s %s（工作目录 %s）", p.Name, p.Exe, strings.Join(p.Args, " "), filepath.Dir(p.Exe))

	// ShellExecute 只保证“系统受理了请求”，参数错误等情况下程序会立刻退出，
	// 这里短暂观察进程是否真的存活，失败时把原因回传给页面
	if !platform.WaitProcess(p.Exe, 600*time.Millisecond) {
		return fmt.Errorf("启动后未检测到进程：可能参数有误或程序立即退出%s，详见 app.log", argHint(p.Args))
	}
	time.Sleep(startupGrace - 600*time.Millisecond)
	s.mu.Lock()
	stopping := in.stopping
	s.mu.Unlock()
	if stopping {
		return nil
	}
	if len(platform.ProcessPIDs(p.Exe)) == 0 {
		return fmt.Errorf("程序启动后立即退出：多半是启动参数有误%s，详见 app.log", argHint(p.Args))
	}
	return nil
}

// argHint 提示容易写错的参数写法：数组里每个元素都是一个完整参数
func argHint(args []string) string {
	for _, a := range args {
		if strings.ContainsAny(a, " \t") {
			return fmt.Sprintf("；参数 %q 含空格，会被当作单个参数，请确认是否应拆成多项", a)
		}
	}
	return ""
}

// startProgramExec 直接创建进程，用于需要注入环境变量的程序
func (s *Server) startProgramExec(in *instance) error {
	p := in.p

	cmd := exec.Command(p.Exe, p.Args...)
	// 工作目录与资源管理器双击一致：程序所在目录
	cmd.Dir = filepath.Dir(p.Exe)
	if len(p.Env) > 0 {
		cmd.Env = os.Environ()
		for k, v := range p.Env {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
	}
	platform.PrepareChild(cmd, p.Exe)
	log.Printf("应用 %s 启动: %s %s（工作目录 %s，已注入环境变量）", p.Name, p.Exe, strings.Join(p.Args, " "), filepath.Dir(p.Exe))

	if err := cmd.Start(); err != nil {
		if platform.NeedsElevation(err) {
			// 目标程序带 requireAdministrator 清单，直接 Start 会被系统拒绝，
			// 改走 UAC 授权，与双击时的行为一致
			if eerr := platform.RunElevated(p.Exe, p.Args, filepath.Dir(p.Exe)); eerr != nil {
				return fmt.Errorf("需要管理员权限: %w", eerr)
			}
			log.Printf("应用 %s 需要管理员权限，已请求系统授权启动", p.Name)
			return nil
		}
		return fmt.Errorf("无法启动: %w", err)
	}

	pr := &procState{cmd: cmd, done: make(chan struct{})}
	s.mu.Lock()
	in.proc = pr
	s.mu.Unlock()

	go s.watch(in, pr)
	log.Printf("应用 %s 进程已启动 (pid %d)", p.Name, cmd.Process.Pid)

	// 观察期内没有退出才认为启动成功；立即崩溃的进程往往是缺依赖或配置错误
	select {
	case <-pr.done:
		s.mu.Lock()
		stopping := in.stopping
		s.mu.Unlock()
		if stopping {
			return nil
		}
		// 主进程退出但同名进程还在，说明程序把工作交给了已在运行的实例（单实例机制）
		s.forgetProcs()
		if len(platform.ProcessPIDs(p.Exe)) > 0 {
			log.Printf("应用 %s 主进程已退出，但检测到同名进程仍在运行", p.Name)
			return nil
		}
		return fmt.Errorf("启动后立即退出（%s）%s", exitReason(pr), argHint(p.Args))
	case <-time.After(startupGrace):
	}
	return nil
}

// exitReason 把退出信息整理成可读文本
func exitReason(pr *procState) string {
	if pr.cmd.ProcessState == nil {
		return "未知原因"
	}
	return fmt.Sprintf("退出码 %d", pr.cmd.ProcessState.ExitCode())
}

// watch 回收进程并同步运行态，进程退出后卡片自动回到未启动
func (s *Server) watch(in *instance, pr *procState) {
	err := pr.cmd.Wait()
	close(pr.done)

	s.mu.Lock()
	if in.proc == pr {
		in.proc, in.stopping = nil, false
	}
	s.mu.Unlock()

	if err != nil {
		log.Printf("应用 %s 进程已退出（%s）", in.p.Name, exitReason(pr))
	} else {
		log.Printf("应用 %s 进程已退出", in.p.Name)
	}
}

// stop 停止指定项目：静态站点关闭监听，程序结束进程树
func (s *Server) stop(in *instance) {
	s.mu.Lock()
	srv, pr := in.srv, in.proc
	s.mu.Unlock()

	if srv != nil {
		_ = srv.Close()
		s.mu.Lock()
		if in.srv == srv {
			in.srv, in.port, in.url = nil, 0, ""
		}
		in.startErr = ""
		s.mu.Unlock()
		log.Printf("应用 %s 已停止", in.p.Name)
		return
	}

	if in.p.Kind != project.KindProgram {
		return
	}

	s.mu.Lock()
	in.stopping = true
	s.mu.Unlock()

	if pr != nil {
		platform.KillTree(pr.cmd.Process.Pid)
		select {
		case <-pr.done:
		case <-time.After(3 * time.Second):
			log.Printf("应用 %s 进程未能及时退出 (pid %d)", in.p.Name, pr.cmd.Process.Pid)
		}
	} else {
		// 不是启动器拉起的实例（例如用户自己双击打开），按 exe 名结束
		for _, pid := range platform.ProcessPIDs(in.p.Exe) {
			platform.KillTree(pid)
		}
	}
	s.forgetProcs()

	s.mu.Lock()
	in.startErr, in.stopping = "", false
	s.mu.Unlock()
	log.Printf("应用 %s 已停止", in.p.Name)
}

// StopAll 停止所有已启动的项目，供托盘菜单与退出流程调用
func (s *Server) StopAll() {
	for _, in := range s.snapshot() {
		s.stop(in)
	}
}

// reservePort 占用一个端口并返回监听器：want 为 0 时从端口池里取
func reservePort(want int) (net.Listener, error) {
	if want > 0 {
		return net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", want))
	}
	for port := portStart; port <= portEnd; port++ {
		if ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port)); err == nil {
			return ln, nil
		}
	}
	return nil, fmt.Errorf("%d-%d 端口均被占用", portStart, portEnd)
}
