package launcher

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"portico/internal/project"
)

func TestPageProblemAndIcon(t *testing.T) {
	base := t.TempDir()

	bad := filepath.Join(base, project.DirName, "坏项目")
	if err := os.MkdirAll(bad, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bad, "readme.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	good := filepath.Join(base, project.DirName, "好项目")
	if err := os.MkdirAll(good, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []struct{ name, content string }{
		{"index.html", "ok"},
		{"icon.png", "png"},
	} {
		if err := os.WriteFile(filepath.Join(good, f.name), []byte(f.content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	store, err := project.OpenAt(base)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, project.ConfigFile), []byte(`{"link:demo":{"name":"公司官网","url":"https://example.com"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	list, err := store.Scan()
	if err != nil {
		t.Fatal(err)
	}

	s := New(store, list)

	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, httptest.NewRequest("GET", "http://127.0.0.1:18080/", nil))
	if rec.Code != 200 {
		t.Fatalf("首页状态码 %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"缺少 index.html", "/icon/", "打开目录", "v1.0.0", "外部链接", "添加链接", "https://example.com"} {
		if !strings.Contains(body, want) {
			t.Errorf("页面缺少 %q", want)
		}
	}

	rec = httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, httptest.NewRequest("GET", "http://127.0.0.1:18080/icon/好项目", nil))
	if rec.Code != 200 || rec.Body.String() != "png" {
		t.Errorf("图标请求失败: code=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestAddAndDeleteLink(t *testing.T) {
	base := t.TempDir()
	store, err := project.OpenAt(base)
	if err != nil {
		t.Fatal(err)
	}
	s := New(store, nil)

	req := httptest.NewRequest("POST", "http://127.0.0.1:18080/link", strings.NewReader("name=%E5%AE%98%E7%BD%91&url=example.com"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("添加链接应重定向，实际 %d", rec.Code)
	}

	list, err := store.Scan()
	if err != nil {
		t.Fatal(err)
	}
	var link *project.Project
	for _, p := range list {
		if p.Kind == project.KindLink {
			link = p
		}
	}
	if link == nil || link.Name != "官网" || link.URL != "https://example.com" {
		t.Fatalf("链接未写入: %+v", link)
	}

	rec = httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, httptest.NewRequest("POST", "http://127.0.0.1:18080/unlink/"+link.Key, nil))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("删除链接应重定向，实际 %d", rec.Code)
	}
	list, _ = store.Scan()
	for _, p := range list {
		if p.Kind == project.KindLink {
			t.Errorf("链接应已删除: %+v", p)
		}
	}
}

func TestChooseExeFromPage(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, project.DirName, "工具")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a.exe", "b.exe"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	store, err := project.OpenAt(base)
	if err != nil {
		t.Fatal(err)
	}
	list, err := store.Scan()
	if err != nil {
		t.Fatal(err)
	}
	s := New(store, list)

	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, httptest.NewRequest("GET", "http://127.0.0.1:18080/", nil))
	body := rec.Body.String()
	if !strings.Contains(body, "exe-pick") || !strings.Contains(body, "a.exe") {
		t.Fatalf("页面应提供 exe 选择框: %s", body)
	}

	req := httptest.NewRequest("POST", "http://127.0.0.1:18080/exe/工具", strings.NewReader("exe=a.exe"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("选择程序应重定向，实际 %d: %s", rec.Code, rec.Body.String())
	}

	raw, err := os.ReadFile(filepath.Join(base, project.ConfigFile))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"exe": "a.exe"`) {
		t.Errorf("apps.json 未写入主程序: %s", raw)
	}

	list, _ = store.Scan()
	if len(list) != 1 || list[0].Kind != project.KindProgram || filepath.Base(list[0].Exe) != "a.exe" {
		t.Errorf("重新扫描后应识别为程序: %+v", list)
	}
}
