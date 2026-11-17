package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScan(t *testing.T) {
	root := t.TempDir()

	write := func(rel, content string) {
		t.Helper()
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	write("订单系统/index.html", "ok")
	write("报表中心/index.html", "ok")
	write("空目录/readme.md", "no index")
	write("README.txt", "说明文件不是项目")

	store := newTestStore(t, root)
	writeFile(t, store.config, `{"报表中心":{"name":"经营报表"}}`)
	list, err := store.Scan()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 {
		t.Fatalf("应发现 3 个目录，实际 %d 个", len(list))
	}

	names := map[string]bool{}
	for _, p := range list {
		names[p.Name] = true
		if p.Dir == "" {
			t.Errorf("%s 缺少目录", p.Name)
		}
		if p.Key == "空目录" && p.Problem != "缺少 index.html" {
			t.Errorf("空目录应标记问题，实际 %q", p.Problem)
		}
		if p.Key != "空目录" && p.Problem != "" {
			t.Errorf("%s 不应有问题标记: %q", p.Key, p.Problem)
		}
	}
	if !names["订单系统"] || !names["经营报表"] {
		t.Errorf("项目名称解析错误: %v", names)
	}
}

func TestScanProgram(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "订单系统", "server.exe"), "")
	writeFile(t, filepath.Join(root, "报表中心", "api.exe"), "")
	writeFile(t, filepath.Join(root, "报表中心", "app.json"), `{
		"args": ["--port", "{port}"],
		"port": 9001,
		"env": {"APP_ENV": "prod"}
	}`)
	// 目录里同时有静态入口时按静态项目处理
	writeFile(t, filepath.Join(root, "混合", "index.html"), "ok")
	writeFile(t, filepath.Join(root, "混合", "other.exe"), "")

	store := newTestStore(t, root)
	writeFile(t, store.config, `{"报表中心":{"name":"经营报表"}}`)
	list, err := store.Scan()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 {
		t.Fatalf("应发现 3 个项目，实际 %d 个", len(list))
	}

	byKey := map[string]*Project{}
	for _, p := range list {
		byKey[p.Key] = p
	}

	order := byKey["订单系统"]
	if order == nil || order.Kind != KindProgram {
		t.Fatalf("订单系统应识别为程序型: %+v", order)
	}
	if filepath.Base(order.Exe) != "server.exe" {
		t.Errorf("exe 定位错误: %s", order.Exe)
	}

	report := byKey["报表中心"]
	if report == nil || report.Kind != KindProgram {
		t.Fatalf("报表中心应识别为程序型: %+v", report)
	}
	if report.Name != "经营报表" || len(report.Args) != 2 {
		t.Errorf("app.json 未正确解析: %+v", report)
	}
	if report.Env["APP_ENV"] != "prod" {
		t.Errorf("环境变量未正确解析: %v", report.Env)
	}

	if mixed := byKey["混合"]; mixed == nil || mixed.Kind != KindStatic {
		t.Errorf("同时含 index.html 时应当作静态项目: %+v", mixed)
	}
}

func TestFindExeRequiresUnique(t *testing.T) {
	dir := t.TempDir()
	if got := findExe(dir, ""); got != "" {
		t.Errorf("空目录不应返回可执行文件: %s", got)
	}
	writeFile(t, filepath.Join(dir, "a.exe"), "")
	writeFile(t, filepath.Join(dir, "b.exe"), "")
	if got := findExe(dir, ""); got != "" {
		t.Errorf("多个 exe 且未指定时应放弃识别: %s", got)
	}
	if got := findExe(dir, "b.exe"); filepath.Base(got) != "b.exe" {
		t.Errorf("指定 exe 后应能定位: %s", got)
	}
}

func TestFindExePickMain(t *testing.T) {
	named := filepath.Join(t.TempDir(), "MyGo Music")
	writeFile(t, filepath.Join(named, "MyGo Music.exe"), "")
	writeFile(t, filepath.Join(named, "Uninstall.exe"), "")
	if got := findExe(named, ""); filepath.Base(got) != "MyGo Music.exe" {
		t.Errorf("应优先选择与目录同名的 exe，实际 %s", got)
	}

	other := filepath.Join(t.TempDir(), "工具")
	writeFile(t, filepath.Join(other, "main.exe"), "")
	writeFile(t, filepath.Join(other, "Uninstall.exe"), "")
	if got := findExe(other, ""); filepath.Base(got) != "main.exe" {
		t.Errorf("排除卸载程序后应唯一确定，实际 %s", got)
	}
}

func TestOpenAt(t *testing.T) {
	base := t.TempDir()

	store, err := OpenAt(base)
	if err != nil {
		t.Fatal(err)
	}
	if store.Root() != filepath.Join(base, DirName) {
		t.Errorf("apps 目录位置错误: %s", store.Root())
	}
	if store.config != filepath.Join(base, ConfigFile) {
		t.Errorf("配置应位于 exe 同目录: %s", store.config)
	}
	if _, err := os.Stat(filepath.Join(base, DirName, "README.txt")); err != nil {
		t.Errorf("应生成 README.txt: %v", err)
	}
	if _, err := os.Stat(store.config); !os.IsNotExist(err) {
		t.Errorf("没有展示配置时不应生成 apps.json，实际 err=%v", err)
	}
}

func newTestStore(t *testing.T, root string) *Store {
	t.Helper()
	return &Store{root: root, config: filepath.Join(t.TempDir(), ConfigFile)}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestNoteAndOrder(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "对账工具")
	writeFile(t, filepath.Join(dir, "index.html"), "ok")

	store := newTestStore(t, root)
	writeFile(t, store.config, `{
		"对账工具": {
			"name": "对账",
			"note": "给财务部用的对账工具",
			"pin": true,
			"order": 20
		}
	}`)

	list, err := store.Scan()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("应发现 1 个项目，实际 %d 个", len(list))
	}
	p := list[0]
	if p.Note != "给财务部用的对账工具" {
		t.Errorf("说明读取错误，实际 %q", p.Note)
	}
	if p.Name != "对账" || !p.Pin || p.Order != 20 {
		t.Errorf("集中配置未生效: name=%q pin=%v order=%d", p.Name, p.Pin, p.Order)
	}

	// 修改说明与排序写进 apps.json
	p.Pin, p.Order = false, 10
	if err := store.SaveNote(p, " 新的说明 "); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveOrder(p); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(store.config)
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	for _, want := range []string{`"note": "新的说明"`, `"order": 10`} {
		if !strings.Contains(text, want) {
			t.Errorf("apps.json 缺少 %s:\n%s", want, text)
		}
	}
	if strings.Contains(text, `"pin"`) {
		t.Errorf("取消置顶后不应保留 pin:\n%s", text)
	}

	// 清空说明后 note 字段消失
	if err := store.SaveNote(p, "   "); err != nil {
		t.Fatal(err)
	}
	b, err = os.ReadFile(store.config)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), `"note"`) {
		t.Errorf("清空说明后不应保留 note:\n%s", b)
	}

	// 只有说明的应用，清空后整个条目被移除
	other := filepath.Join(root, "临时工具")
	writeFile(t, filepath.Join(other, "index.html"), "ok")
	otherP := &Project{Key: "临时工具", Name: "临时工具", Dir: other}
	if err := store.SaveNote(otherP, "临时"); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(store.config)
	if !strings.Contains(string(b), `"临时工具"`) {
		t.Errorf("保存说明后应写入条目:\n%s", b)
	}
	if err := store.SaveNote(otherP, ""); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(store.config)
	if strings.Contains(string(b), `"临时工具"`) {
		t.Errorf("清空说明后不应保留空条目:\n%s", b)
	}
}

func TestLinkEntry(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "订单系统", "index.html"), "ok")

	store := newTestStore(t, root)
	key, err := store.AddLink("", "https://example.com/a")
	if err != nil {
		t.Fatal(err)
	}
	if !LinkKey(key) {
		t.Fatalf("链接 key 应带前缀: %s", key)
	}

	cfg, err := readConfig(store.config)
	if err != nil {
		t.Fatal(err)
	}
	if cfg[key].Name != "example.com" || cfg[key].URL != "https://example.com/a" {
		t.Errorf("链接条目保存错误: %+v", cfg[key])
	}

	list, err := store.Scan()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("应发现 1 个目录项目 + 1 个链接，实际 %d 个", len(list))
	}
	var link *Project
	for _, p := range list {
		if p.Key == key {
			link = p
		}
	}
	if link == nil || link.Kind != KindLink {
		t.Fatalf("链接条目未识别为 KindLink: %+v", link)
	}
	if link.Name != "example.com" || link.URL != "https://example.com/a" || link.Dir != "" {
		t.Errorf("链接项目字段错误: %+v", link)
	}

	if err := store.UpdateLink(key, "官网", "http://example.org"); err != nil {
		t.Fatal(err)
	}
	cfg, _ = readConfig(store.config)
	if cfg[key].Name != "官网" || cfg[key].URL != "http://example.org" {
		t.Errorf("修改链接未生效: %+v", cfg[key])
	}

	if err := store.DeleteLink(key); err != nil {
		t.Fatal(err)
	}
	cfg, _ = readConfig(store.config)
	if len(cfg) != 0 {
		t.Errorf("删除链接后不应残留条目: %+v", cfg)
	}
}

func TestNormalizeLink(t *testing.T) {
	ok := map[string]string{
		"example.com":           "https://example.com",
		" https://a.cn/x?y=1 ":  "https://a.cn/x?y=1",
		"http://127.0.0.1:9000": "http://127.0.0.1:9000",
	}
	for in, want := range ok {
		got, err := NormalizeLink(in)
		if err != nil || got != want {
			t.Errorf("NormalizeLink(%q) = %q, %v；期望 %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "   ", "ftp://a.cn", "https://"} {
		if _, err := NormalizeLink(bad); err == nil {
			t.Errorf("非法地址 %q 应报错", bad)
		}
	}
}
