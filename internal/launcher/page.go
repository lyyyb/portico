package launcher

import (
	"hash/fnv"
	"net/url"
	"sort"

	"portico/internal/project"
)

type pageApp struct {
	Key       string
	Name      string
	Note      string
	Link      string
	StartLink string
	StopLink  string
	NoteLink  string
	PinLink   string
	MoveLink  string
	ExeLink   string
	Initial   string
	Hue       int
	Index     int
	Started   bool
	Pin       bool
	Port      int
	Program   bool
	External  bool // 外部链接卡片

	Exes []string // 多个 exe 时供用户选择主程序

	Dir     string // 项目目录，用于提示
	Icon    string // 图标地址，没有图标时为空
	URL     string // 链接卡片的跳转地址
	Host    string // 链接卡片的域名，用于副标题
	Problem string // 无法启动的原因，空表示正常
}

type pageData struct {
	Apps     []pageApp
	Total    int
	Started  int
	AppsPath string
	Version  string
}

// ordered 返回排序后的副本：置顶优先，其次 order，再按是否运行、名称
func ordered(instances []*instance, running func(*instance) bool) []*instance {
	list := make([]*instance, len(instances))
	copy(list, instances)
	sort.SliceStable(list, func(i, j int) bool {
		a, b := list[i].p, list[j].p
		if a.Pin != b.Pin {
			return a.Pin
		}
		ao, bo := orderOf(a), orderOf(b)
		if ao != bo {
			return ao < bo
		}
		ai, bj := running(list[i]), running(list[j])
		if ai != bj {
			return ai
		}
		return a.Name < b.Name
	})
	return list
}

// orderOf 把未设置的 order 视为最后，避免它排到显式排序之前
func orderOf(p *project.Project) int {
	if p.Order == 0 {
		return 1 << 30
	}
	return p.Order
}

// buildPage 把运行态转换成页面数据，顺序与卡片展示一致
func buildPage(instances []*instance, appsPath string, running func(*instance) bool) pageData {
	rows := make([]pageApp, 0, len(instances))
	started := 0
	for _, in := range ordered(instances, running) {
		on := running(in)
		if on {
			started++
		}
		key := url.PathEscape(in.p.Key)
		icon := ""
		if in.p.Dir != "" && project.IconFile(in.p.Dir) != "" {
			icon = "/icon/" + key
		}
		host := ""
		if u, err := url.Parse(in.p.URL); err == nil {
			host = u.Host
		}
		rows = append(rows, pageApp{
			Key:       in.p.Key,
			Name:      in.p.Name,
			Note:      in.p.Note,
			Link:      "/open/" + key,
			StartLink: "/start/" + key,
			StopLink:  "/stop/" + key,
			NoteLink:  "/note/" + key,
			PinLink:   "/pin/" + key,
			MoveLink:  "/move/" + key,
			ExeLink:   "/exe/" + key,
			Initial:   initial(in.p.Name),
			Hue:       hue(in.p.Name),
			Started:   on,
			Pin:       in.p.Pin,
			Port:      in.port,
			Program:   in.p.Kind == project.KindProgram,
			External:  in.p.Kind == project.KindLink,
			Exes:      in.p.Exes,
			Dir:       in.p.Dir,
			Icon:      icon,
			URL:       in.p.URL,
			Host:      host,
			Problem:   in.p.Problem,
		})
	}
	for i := range rows {
		rows[i].Index = i
	}
	return pageData{Apps: rows, Total: len(rows), Started: started, AppsPath: appsPath}
}

// initial 取名称首字，作为卡片上的字母块
func initial(name string) string {
	for _, r := range name {
		return string(r)
	}
	return "?"
}

// hue 由名称派生一个稳定的色相，让每个项目有自己的识别色
func hue(name string) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(name))
	return int(h.Sum32() % 360)
}
