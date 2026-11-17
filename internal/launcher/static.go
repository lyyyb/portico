package launcher

import (
	"io/fs"
	"net/http"
	"os"
	"path"
	"strings"
)

// assetExt 是明确按静态资源对待的扩展名，缺失时返回 404 而不是回退到页面
var assetExt = map[string]bool{
	".js": true, ".mjs": true, ".css": true, ".map": true, ".json": true,
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".svg": true,
	".webp": true, ".ico": true, ".bmp": true, ".avif": true,
	".woff": true, ".woff2": true, ".ttf": true, ".otf": true, ".eot": true,
	".mp4": true, ".webm": true, ".mp3": true, ".wav": true, ".wasm": true,
	".pdf": true, ".txt": true, ".xml": true, ".csv": true, ".zip": true,
}

func appHandler(dir string) http.Handler {
	files := http.FileServer(http.FS(spaFS{fsys: os.DirFS(dir)}))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", cacheHeader(r.URL.Path))
		files.ServeHTTP(w, r)
	})
}

// cacheHeader 带 hash 的资源长缓存，入口页不缓存
func cacheHeader(p string) string {
	switch {
	case p == "/" || strings.HasSuffix(p, "/") || path.Ext(p) == "":
		return "no-store"
	case strings.HasPrefix(p, "/assets/"):
		return "public, max-age=31536000, immutable"
	default:
		return "no-cache"
	}
}

// spaFS 在找不到文件时回退到 index.html，兼容前端路由
type spaFS struct{ fsys fs.FS }

func (s spaFS) Open(name string) (fs.File, error) {
	f, err := s.fsys.Open(name)
	if err == nil {
		return f, nil
	}
	// 只把明确的静态资源视为真 404，其余路径交给前端路由处理
	// 这样 /user/1.2 这类带点的路由也能正常回退
	if strings.HasPrefix(name, "assets/") || assetExt[strings.ToLower(path.Ext(name))] {
		return nil, fs.ErrNotExist
	}
	return s.fsys.Open("index.html")
}
