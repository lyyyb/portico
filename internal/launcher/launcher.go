package launcher

import (
	"io"
	"log"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"portico/internal/platform"
	"portico/internal/project"
	"portico/internal/version"
)

// Addr 是启动器自身监听的地址
const Addr = "127.0.0.1:18080"

const (
	portStart = 18081
	portEnd   = 18100
)

// instance 是一个项目的运行态，项目本身保持只读
type instance struct {
	p       *project.Project
	startMu sync.Mutex // 串行化同一项目的启动

	srv  *http.Server // 静态站点
	proc *procState   // 本地程序
	port int          // 仅静态站点使用
	url  string       // 仅静态站点使用

	starting bool   // 启动流程进行中
	startErr string // 最近一次启动失败原因
	stopping bool   // 用户主动停止，用于区分"启动后立即退出"
}

// live 表示启动器自己托管的服务或进程仍在运行
func (in *instance) live() bool {
	return in.srv != nil || in.proc != nil
}

// Server 负责启动器页面与各项目的按需启停
type Server struct {
	mu        sync.Mutex
	store     *project.Store
	instances []*instance

	// 进程列表缓存：程序型项目靠它判断"这个 exe 是否在跑"
	procMu  sync.Mutex
	procs   map[string][]int
	procsAt time.Time
}

func URL() string {
	return "http://" + Addr + "/"
}

// Alive 探测 18080 上跑的是不是自己
func Alive() bool {
	client := &http.Client{Timeout: 500 * time.Millisecond}
	resp, err := client.Get(URL() + "healthz")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func New(store *project.Store, projects []*project.Project) *Server {
	s := &Server{store: store}
	for _, p := range projects {
		s.instances = append(s.instances, &instance{p: p})
	}
	return s
}

// running 表示项目在运行：静态站点看监听，程序型看进程是否存活
func (s *Server) running(in *instance) bool {
	if in.live() {
		return true
	}
	return in.p.Kind == project.KindProgram && len(s.pidsOf(in.p.Exe)) > 0
}

// pidsOf 按 exe 文件名查找系统里的进程，带 500ms 缓存避免每次轮询都全量扫描
func (s *Server) pidsOf(exe string) []int {
	s.procMu.Lock()
	defer s.procMu.Unlock()

	if s.procs == nil || time.Since(s.procsAt) > 500*time.Millisecond {
		if all, err := platform.Processes(); err == nil {
			s.procs, s.procsAt = all, time.Now()
		}
	}
	return s.procs[strings.ToLower(filepath.Base(exe))]
}

// forgetProcs 让下一次查询重新扫描进程列表
func (s *Server) forgetProcs() {
	s.procMu.Lock()
	s.procs, s.procsAt = nil, time.Time{}
	s.procMu.Unlock()
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = io.WriteString(w, "ok")
	})
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		s.render(w, r)
	})
	mux.HandleFunc("GET /open/{key}", s.openApp)
	mux.HandleFunc("GET /icon/{key}", s.iconApp)
	mux.HandleFunc("POST /open-dir", s.openDir)
	mux.HandleFunc("POST /start/{key}", s.startApp)
	mux.HandleFunc("POST /stop/{key}", s.stopApp)
	mux.HandleFunc("POST /exe/{key}", s.chooseExe)
	mux.HandleFunc("POST /note/{key}", s.noteApp)
	mux.HandleFunc("POST /pin/{key}", s.pinApp)
	mux.HandleFunc("POST /move/{key}", s.moveApp)
	mux.HandleFunc("POST /link", s.addLink)
	mux.HandleFunc("POST /link/{key}", s.editLink)
	mux.HandleFunc("POST /unlink/{key}", s.deleteLink)
	mux.HandleFunc("GET /api/status", s.statusAll)
	return guard(mux)
}

// guard 只接受本机 Host，并拒绝跨站写操作
func guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !localHost(r.Host) {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
			http.Error(w, "forbidden origin", http.StatusForbidden)
			return
		}
		if r.Method == http.MethodPost {
			if origin := r.Header.Get("Origin"); origin != "" && origin != "http://"+r.Host {
				http.Error(w, "forbidden origin", http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func localHost(hostport string) bool {
	host, _, err := net.SplitHostPort(hostport)
	if err != nil {
		host = hostport
	}
	switch strings.Trim(host, "[]") {
	case "127.0.0.1", "localhost", "::1":
		return true
	}
	return false
}

// sync 重新扫描 apps 目录，并保留已启动项目的运行态
func (s *Server) sync() {
	projects, err := s.store.Scan()
	if err != nil {
		log.Printf("扫描应用失败: %v", err)
		return
	}

	s.mu.Lock()
	byKey := make(map[string]*instance, len(s.instances))
	for _, in := range s.instances {
		byKey[in.p.Key] = in
	}

	next := make([]*instance, 0, len(projects))
	for _, p := range projects {
		if in, ok := byKey[p.Key]; ok {
			in.p = p
			delete(byKey, p.Key)
			next = append(next, in)
			continue
		}
		next = append(next, &instance{p: p})
	}
	removed := make([]*instance, 0, len(byKey))
	for _, in := range byKey {
		removed = append(removed, in)
	}
	s.instances = next
	s.mu.Unlock()

	for _, in := range removed {
		log.Printf("应用 %s 已移除", in.p.Name)
		s.stop(in)
	}
}

// ensure 首次访问时按需启动，之后复用
func (s *Server) ensure(in *instance) error {
	in.startMu.Lock()
	defer in.startMu.Unlock()

	s.mu.Lock()
	running := s.running(in)
	s.mu.Unlock()
	if running {
		return nil
	}
	if in.p.Kind == project.KindProgram {
		return s.startProgram(in)
	}
	return s.startStatic(in)
}

// startAsync 在后台完成启动，用 starting 标记避免重复启动
func (s *Server) startAsync(in *instance) {
	s.mu.Lock()
	if s.running(in) || in.starting {
		s.mu.Unlock()
		return
	}
	in.starting, in.startErr, in.stopping = true, "", false
	s.mu.Unlock()

	err := s.ensure(in)

	s.mu.Lock()
	in.starting = false
	if err != nil {
		in.startErr = err.Error()
	}
	s.mu.Unlock()
}

func (s *Server) find(key string) *instance {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, in := range s.instances {
		if in.p.Key == key {
			return in
		}
	}
	return nil
}

func (s *Server) snapshot() []*instance {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*instance, len(s.instances))
	copy(out, s.instances)
	return out
}

func (s *Server) render(w http.ResponseWriter, _ *http.Request) {
	s.sync()

	s.mu.Lock()
	data := buildPage(s.instances, s.store.Root(), s.running)
	s.mu.Unlock()
	data.Version = version.Version

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := launcherTemplate.Execute(w, data); err != nil {
		log.Printf("渲染启动器失败: %v", err)
	}
}

// RunningApps 返回运行中的应用，供托盘菜单展示
func (s *Server) RunningApps() []platform.TrayApp {
	s.mu.Lock()
	defer s.mu.Unlock()

	var out []platform.TrayApp
	for _, in := range s.instances {
		if s.running(in) {
			out = append(out, platform.TrayApp{
				Key:  in.p.Key,
				Name: in.p.Name,
				Web:  in.p.Kind != project.KindProgram,
			})
		}
	}
	return out
}

// OpenApp 打开某个运行中应用的界面：桌面程序把已有窗口切到前台，
// 没有可显示窗口时不做任何跳转；静态站点打开本地服务
func (s *Server) OpenApp(key string) {
	var pids []int
	target := ""
	isProgram := false

	s.mu.Lock()
	for _, in := range s.instances {
		if in.p.Key != key {
			continue
		}
		if in.p.Kind == project.KindProgram {
			isProgram = true
			if in.proc != nil {
				pids = append(pids, in.proc.cmd.Process.Pid)
			}
			pids = append(pids, s.pidsOf(in.p.Exe)...)
		} else if in.url != "" {
			target = in.url
		}
		break
	}
	s.mu.Unlock()

	if isProgram {
		if !platform.ActivateWindows(pids) {
			log.Printf("应用 %s 没有可显示的窗口", key)
		}
		return
	}
	if target != "" {
		platform.OpenBrowser(target)
	}
}

// StopApp 停止单个应用
func (s *Server) StopApp(key string) {
	s.sync()
	if in := s.find(key); in != nil {
		s.stop(in)
	}
}
