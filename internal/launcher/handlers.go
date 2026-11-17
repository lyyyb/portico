package launcher

import (
	"net/http"
	"strings"

	"portico/internal/platform"
	"portico/internal/project"
)

// route 刷新项目列表并定位路径里 key 对应的项目
func (s *Server) route(r *http.Request) *instance {
	s.sync()
	return s.find(r.PathValue("key"))
}

// openApp 只用于静态项目：启动站点并跳转
func (s *Server) openApp(w http.ResponseWriter, r *http.Request) {
	in := s.route(r)
	if in == nil {
		http.NotFound(w, r)
		return
	}
	if in.p.Kind == project.KindLink {
		http.Redirect(w, r, in.p.URL, http.StatusFound)
		return
	}
	if in.p.Kind == project.KindProgram {
		// 程序型项目不跳转，统一回到启动器页面看状态
		home(w, r)
		return
	}
	if err := s.ensure(in); err != nil {
		writeError(w, "无法启动该项目", err)
		return
	}

	s.mu.Lock()
	target := in.url
	s.mu.Unlock()
	http.Redirect(w, r, target, http.StatusFound)
}

// startApp 启动程序型项目，随后回到启动器页面展示运行状态
func (s *Server) startApp(w http.ResponseWriter, r *http.Request) {
	in := s.route(r)
	if in == nil {
		http.NotFound(w, r)
		return
	}
	if in.p.Kind == project.KindProgram {
		go s.startAsync(in)
	} else {
		_ = s.ensure(in)
	}
	home(w, r)
}

func (s *Server) stopApp(w http.ResponseWriter, r *http.Request) {
	in := s.route(r)
	if in == nil {
		http.NotFound(w, r)
		return
	}
	s.stop(in)
	home(w, r)
}

// noteApp 保存应用说明到启动器同目录的 apps.json
func (s *Server) noteApp(w http.ResponseWriter, r *http.Request) {
	in := s.route(r)
	if in == nil {
		http.NotFound(w, r)
		return
	}

	text := strings.TrimSpace(r.FormValue("text"))
	if err := s.store.SaveNote(in.p, text); err != nil {
		writeError(w, "无法保存说明", err)
		return
	}

	s.mu.Lock()
	in.p.Note = text
	s.mu.Unlock()
	home(w, r)
}

// pinApp 切换置顶，结果写入启动器同目录的 apps.json
func (s *Server) pinApp(w http.ResponseWriter, r *http.Request) {
	in := s.route(r)
	if in == nil {
		http.NotFound(w, r)
		return
	}
	if err := s.togglePin(in); err != nil {
		writeError(w, "无法保存排序", err)
		return
	}
	home(w, r)
}

// moveApp 上移/下移一位，重新编号后写回 apps.json
func (s *Server) moveApp(w http.ResponseWriter, r *http.Request) {
	in := s.route(r)
	if in == nil {
		http.NotFound(w, r)
		return
	}
	delta := -1
	if r.FormValue("dir") == "down" {
		delta = 1
	}
	s.moveInstance(in, delta)
	home(w, r)
}

// iconApp 返回项目目录里的图标文件
func (s *Server) iconApp(w http.ResponseWriter, r *http.Request) {
	in := s.route(r)
	if in == nil || in.p.Dir == "" {
		http.NotFound(w, r)
		return
	}
	p := project.IconFile(in.p.Dir)
	if p == "" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "max-age=60")
	http.ServeFile(w, r, p)
}

// addLink 新增一个外部链接卡片
func (s *Server) addLink(w http.ResponseWriter, r *http.Request) {
	target, err := project.NormalizeLink(r.FormValue("url"))
	if err != nil {
		writeError(w, "无法添加链接", err)
		return
	}
	if _, err := s.store.AddLink(r.FormValue("name"), target); err != nil {
		writeError(w, "无法保存链接", err)
		return
	}
	home(w, r)
}

// editLink 修改链接卡片的名称与地址
func (s *Server) editLink(w http.ResponseWriter, r *http.Request) {
	in := s.route(r)
	if in == nil || in.p.Kind != project.KindLink {
		http.NotFound(w, r)
		return
	}
	target, err := project.NormalizeLink(r.FormValue("url"))
	if err != nil {
		writeError(w, "无法保存链接", err)
		return
	}
	if err := s.store.UpdateLink(in.p.Key, r.FormValue("name"), target); err != nil {
		writeError(w, "无法保存链接", err)
		return
	}
	home(w, r)
}

// deleteLink 删除链接卡片
func (s *Server) deleteLink(w http.ResponseWriter, r *http.Request) {
	in := s.route(r)
	if in == nil || in.p.Kind != project.KindLink {
		http.NotFound(w, r)
		return
	}
	if err := s.store.DeleteLink(in.p.Key); err != nil {
		writeError(w, "无法删除链接", err)
		return
	}
	home(w, r)
}

// openDir 用资源管理器打开 apps 目录
func (s *Server) openDir(w http.ResponseWriter, r *http.Request) {
	platform.OpenFolder(s.store.Root())
	home(w, r)
}

// statusAll 供启动器页面轮询所有项目的状态
func (s *Server) statusAll(w http.ResponseWriter, _ *http.Request) {
	type appState struct {
		Key     string `json:"key"`
		State   string `json:"state"`
		Error   string `json:"error,omitempty"`
		Port    int    `json:"port,omitempty"`
		PID     int    `json:"pid,omitempty"`
		Problem string `json:"problem,omitempty"`
	}

	s.mu.Lock()
	list := make([]appState, 0, len(s.instances))
	for _, in := range s.instances {
		st := appState{Key: in.p.Key, Problem: in.p.Problem}
		switch {
		case in.starting:
			st.State = "starting"
		case s.running(in):
			st.State = "running"
		case in.startErr != "":
			st.State, st.Error = "failed", in.startErr
		default:
			st.State = "idle"
		}

		if in.p.Kind == project.KindProgram {
			// 优先用启动器拉起的进程号，外部启动的实例取同名进程里第一个
			if in.proc != nil {
				st.PID = in.proc.cmd.Process.Pid
			} else if pids := s.pidsOf(in.p.Exe); len(pids) > 0 {
				st.PID = pids[0]
			}
		} else {
			st.Port = in.port
		}

		list = append(list, st)
	}
	s.mu.Unlock()

	writeJSON(w, map[string]any{"apps": list})
}
