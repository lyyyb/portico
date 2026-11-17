package project

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	"portico/internal/platform"
)

// DirName 是存放前端产物或单文件程序的目录，位于可执行文件旁边
const DirName = "apps"

const readmeText = `Portico（应用坞）：一个单文件本地应用启动器。

把静态前端产物或自带后端的单文件程序放在这里，一个项目一个目录。

方式一：静态前端
  子目录内直接包含 index.html（即 dist 里的内容，不要再套一层 dist）

方式二：单文件程序（前端已打包进 exe）
  子目录内放一个 exe，点击卡片时由启动器直接启动它
  可选 app.json（只放启动参数，显示名见下方 apps.json）：
    {
      "exe": "server.exe",
      "args": ["--silent"],
      "env": {"APP_ENV": "prod"}
    }
  - exe：目录内有多个 exe 时指定用哪个（默认优先选与目录同名的，
        其次忽略 Uninstall 等卸载程序；仍无法确定才需要手写）
  - args：附加启动参数
  - env：额外环境变量

外部链接（不建目录）
  只想收藏一个网址时，在启动器页面点"添加链接"，填名称和地址即可。
  链接卡片只做跳转，不参与启动/停止，可置顶排序，保存在 apps.json：
    "link:abc123": {
      "name": "公司官网",
      "url": "https://example.com"
    }

卡片图标（可选）
  项目目录里放 icon.png / icon.ico / favicon.ico 之一即可，
  会显示在卡片左侧；没有则使用名称首字母色块。

无法识别的目录
  缺少 index.html、或有多个 exe 又没在 app.json 指定主程序时，
  卡片仍会显示，但标注原因，按提示补齐文件即可。

应用说明与排序（集中在启动器同目录的 apps.json）
  显示名、说明、置顶、排序统一保存在 Portico 同目录的 apps.json 里，
  不再写进各项目目录，方便统一查看和备份。
  也可以直接在启动器页面点"说明"或"整理"修改，效果相同。
  格式：
    {
      "订单系统": {
        "name": "订单系统",
        "note": "给销售部用的下单系统",
        "pin": true,
        "order": 10
      }
    }
  - name：卡片显示名，不写则用目录名
  - note：卡片说明
  - pin：true 表示置顶
  - order：排序值，越小越靠前

启动器不关心程序监听什么端口，也不跳转页面：点击卡片启动进程，
在启动器页面显示运行状态；进程退出后状态自动回到未启动。

放好后刷新启动器页面即可，无需重启程序。
`

// Kind 表示项目的托管方式
type Kind int

const (
	// KindStatic 是纯静态站点，由启动器直接托管文件
	KindStatic Kind = iota
	// KindProgram 是自带 HTTP 服务的可执行文件，由启动器启动进程
	KindProgram
	// KindLink 是外部链接，只做跳转，不参与启停
	KindLink
)

// Project 是一个可托管的项目
type Project struct {
	Key  string // 目录名，作为唯一标识
	Name string // 展示名
	Dir  string // 项目目录；链接卡片为空

	Note  string // 说明，来自 apps.json
	Pin   bool   // 是否置顶
	Order int    // 排序值，越小越靠前

	Kind Kind

	// 仅 KindProgram 使用
	Exe  string
	Args []string
	Env  map[string]string

	// 仅 KindLink 使用：跳转地址
	URL string

	// Problem 非空表示目录内容不完整，无法启动，内容为原因
	Problem string
}

// Store 是 apps 目录与集中配置的访问入口
type Store struct {
	root   string
	config string // apps.json，位于启动器 exe 同目录

	mu sync.Mutex
}

// Open 确保 apps 目录存在并写入放置说明
func Open() (*Store, error) {
	return OpenAt(platform.BaseDir())
}

// OpenAt 在指定目录下打开 apps 目录与 apps.json，主要用于测试
func OpenAt(base string) (*Store, error) {
	root := filepath.Join(base, DirName)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	readme := filepath.Join(root, "README.txt")
	_ = os.WriteFile(readme, []byte(readmeText), 0o644)

	s := &Store{root: root, config: filepath.Join(base, ConfigFile)}
	return s, nil
}

// Root 返回 apps 目录的绝对路径
func (s *Store) Root() string {
	return s.root
}

// Scan 扫描所有可托管项目，每次调用都会重新读盘
func (s *Store) Scan() ([]*Project, error) {
	entries, err := os.ReadDir(s.root)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var projects []*Project
	cfg, _ := readConfig(s.config)
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		dir := filepath.Join(s.root, e.Name())
		meta := readMeta(dir)
		ce := cfg[e.Name()]

		p := &Project{
			Key:   e.Name(),
			Name:  e.Name(),
			Dir:   dir,
			Note:  ce.Note,
			Pin:   ce.Pin,
			Order: ce.Order,
		}
		if ce.Name != "" {
			p.Name = ce.Name
		}

		switch {
		case hasIndex(dir):
			p.Kind = KindStatic
		default:
			exe := findExe(dir, meta.Exe)
			switch {
			case exe != "":
				p.Kind, p.Exe = KindProgram, exe
				p.Args, p.Env = meta.Args, meta.Env
			case meta.Exe != "":
				p.Problem = "app.json 指定的 exe 不存在：" + meta.Exe
			case len(listExe(dir)) > 1:
				p.Problem = "目录内有多个 exe，请在 app.json 指定主程序"
			default:
				p.Problem = "缺少 index.html"
			}
		}
		projects = append(projects, p)
	}

	// 没有对应目录、但在 apps.json 里写了 url 的条目，作为链接卡片追加
	known := make(map[string]bool, len(projects))
	for _, p := range projects {
		known[p.Key] = true
	}
	for key, ce := range cfg {
		if known[key] || ce.URL == "" {
			continue
		}
		name := ce.Name
		if name == "" {
			name = key
		}
		projects = append(projects, &Project{
			Key:   key,
			Name:  name,
			Note:  ce.Note,
			Pin:   ce.Pin,
			Order: ce.Order,
			Kind:  KindLink,
			URL:   ce.URL,
		})
	}

	sort.Slice(projects, func(i, j int) bool { return projects[i].Name < projects[j].Name })
	return projects, nil
}

func hasIndex(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, "index.html"))
	return err == nil && !info.IsDir()
}

// findExe 定位目录内的可执行文件：优先 app.json 指定，否则要求唯一
func findExe(dir, want string) string {
	if want != "" {
		p := filepath.Join(dir, want)
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p
		}
		return ""
	}

	found := listExe(dir)
	if len(found) == 1 {
		return found[0]
	}
	return pickMainExe(found, filepath.Base(dir))
}

// listExe 列出目录内的全部可执行文件
func listExe(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var found []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		full := filepath.Join(dir, e.Name())
		if isExecutable(full, e.Name()) {
			found = append(found, full)
		}
	}
	return found
}

// pickMainExe 在多个可执行文件里挑选主程序：
// 优先与目录同名的，其次排除卸载程序后若只剩一个就用它
func pickMainExe(found []string, dirName string) string {
	for _, f := range found {
		if strings.EqualFold(exeStem(f), dirName) {
			return f
		}
	}

	var rest []string
	for _, f := range found {
		if !isUninstaller(exeStem(f)) {
			rest = append(rest, f)
		}
	}
	if len(rest) == 1 {
		return rest[0]
	}
	return ""
}

func exeStem(name string) string {
	return strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))
}

func isUninstaller(name string) bool {
	n := strings.ToLower(name)
	return strings.HasPrefix(n, "uninstall") || strings.HasPrefix(n, "unins") || strings.HasPrefix(n, "卸载")
}

func isExecutable(full, name string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(filepath.Ext(name), ".exe")
	}
	info, err := os.Stat(full)
	return err == nil && info.Mode()&0o111 != 0
}

// iconNames 是卡片图标可用的文件名，按优先级排列
var iconNames = []string{"icon.png", "icon.ico", "favicon.png", "favicon.ico", "logo.png"}

// IconFile 返回项目目录里可用的图标文件，没有则返回空
func IconFile(dir string) string {
	for _, name := range iconNames {
		p := filepath.Join(dir, name)
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p
		}
	}
	return ""
}
