package launcher

import (
	"io"
	"strings"
	"testing"
	"testing/fstest"
)

func TestCacheHeader(t *testing.T) {
	cases := map[string]string{
		"/":              "no-store",
		"/user/123":      "no-store",
		"/assets/app.js": "public, max-age=31536000, immutable",
		"/favicon.ico":   "no-cache",
	}
	for p, want := range cases {
		if got := cacheHeader(p); got != want {
			t.Errorf("cacheHeader(%q) = %q, 期望 %q", p, got, want)
		}
	}
}

func TestSpaFallback(t *testing.T) {
	fsys := fstest.MapFS{
		"index.html":    {Data: []byte("<html>index</html>")},
		"assets/app.js": {Data: []byte("js")},
		"logo.png":      {Data: []byte("png")},
	}
	spa := spaFS{fsys: fsys}

	// 前端路由即使带点也应回退到入口页
	for _, name := range []string{"user/1.2", "user/123", "docs/v1.0"} {
		f, err := spa.Open(name)
		if err != nil {
			t.Fatalf("Open(%q) 返回错误: %v", name, err)
		}
		b, _ := io.ReadAll(f)
		_ = f.Close()
		if !strings.Contains(string(b), "index") {
			t.Errorf("Open(%q) 未回退到 index.html", name)
		}
	}

	// 明确的静态资源缺失必须保持 404
	for _, name := range []string{"assets/missing.js", "missing.png", "style.css"} {
		if _, err := spa.Open(name); err == nil {
			t.Errorf("Open(%q) 不应回退到 index.html", name)
		}
	}
}
