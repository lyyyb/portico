package launcher

import (
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
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
		log.Printf("项目 %s 已启动: %s", in.p.Name, url)
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("项目 %s 已停止: %v", in.p.Name, err)
		}
	}()
	return nil
}

func (s *Server) startProgram(in *instance) error {
	p := in.p

	cmd := exec.Command(p.Exe, p.Args...)
	cmd.Dir = p.Dir
	if len(p.Env) > 0 {
		cmd.Env = os.Environ()
		for k, v := range p.Env {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
	}
	platform.PrepareChild(cmd)

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("无法启动: %w", err)
	}

	pr := &procState{cmd: cmd, done: make(chan struct{})}
	s.mu.Lock()
	in.proc = pr
	s.mu.Unlock()

	go s.watch(in, pr)
	log.Printf("项目 %s 进程已启动 (pid %d)", p.Name, cmd.Process.Pid)

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
			log.Printf("项目 %s 主进程已退出，但检测到同名进程仍在运行", p.Name)
			return nil
		}
		return fmt.Errorf("启动后立即退出（%s）", exitReason(pr))
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
		log.Printf("项目 %s 进程已退出（%s）", in.p.Name, exitReason(pr))
	} else {
		log.Printf("项目 %s 进程已退出", in.p.Name)
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
		log.Printf("项目 %s 已停止", in.p.Name)
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
			log.Printf("项目 %s 进程未能及时退出 (pid %d)", in.p.Name, pr.cmd.Process.Pid)
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
	log.Printf("项目 %s 已停止", in.p.Name)
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
