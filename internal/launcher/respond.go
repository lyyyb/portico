package launcher

import (
	"encoding/json"
	"log"
	"net/http"
)

// writeJSON 输出不缓存的 JSON
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("输出 JSON 失败: %v", err)
	}
}

// writeError 记录日志并返回 500，避免每个处理器重复写日志与错误格式
func writeError(w http.ResponseWriter, prefix string, err error) {
	log.Printf("%s: %v", prefix, err)
	http.Error(w, prefix+"："+err.Error(), http.StatusInternalServerError)
}

// home 回到启动器页面，POST 后用 303 避免刷新重复提交
func home(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
