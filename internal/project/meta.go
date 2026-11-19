package project

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ConfigFile 是启动器同目录下的集中配置，保存所有应用的显示名、说明与排序
const ConfigFile = "apps.json"

// linkPrefix 是外部链接在 apps.json 里的 key 前缀，与目录项目区分开
const linkPrefix = "link:"

// appEntry 是 apps.json 里单个应用的配置；url 非空表示这是一个链接卡片
type appEntry struct {
	Name  string `json:"name,omitempty"`
	Note  string `json:"note,omitempty"`
	Pin   bool   `json:"pin,omitempty"`
	Order int    `json:"order,omitempty"`
	URL   string `json:"url,omitempty"`

	// 以下三项是程序型应用的启动参数
	Exe  string            `json:"exe,omitempty"`
	Args []string          `json:"args,omitempty"`
	Env  map[string]string `json:"env,omitempty"`
}

// readConfig 读取集中配置；文件不存在或格式有误时返回空配置
func readConfig(path string) (map[string]appEntry, error) {
	cfg := map[string]appEntry{}
	if path == "" {
		return cfg, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, err
	}
	if len(strings.TrimSpace(string(b))) == 0 {
		return cfg, nil
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		return nil, fmt.Errorf("%s 格式有误: %w", filepath.Base(path), err)
	}
	return cfg, nil
}

func writeConfig(path string, cfg map[string]appEntry) error {
	if len(cfg) == 0 {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

// SaveNote 把说明写入集中配置
func (s *Store) SaveNote(p *Project, text string) error {
	return s.updateEntry(p.Key, func(e *appEntry) {
		e.Note = strings.TrimSpace(text)
	})
}

// SaveOrder 把置顶与排序写入集中配置
func (s *Store) SaveOrder(p *Project) error {
	return s.updateEntry(p.Key, func(e *appEntry) {
		e.Pin = p.Pin
		e.Order = p.Order
	})
}

// SaveExe 把用户选择的主程序写入 apps.json，保留其余展示配置
func (s *Store) SaveExe(p *Project, name string) error {
	rel, ok := safeRel(name)
	if !ok {
		return fmt.Errorf("非法的程序路径: %q", name)
	}

	full := filepath.Join(p.Dir, rel)
	info, err := os.Stat(full)
	if err != nil || info.IsDir() || !isExecutable(full, info.Name()) {
		return fmt.Errorf("目录内没有 %s", name)
	}

	// 统一存成 / 分隔，既好读也避免 JSON 里的反斜杠转义问题
	stored := filepath.ToSlash(rel)
	return s.updateEntry(p.Key, func(e *appEntry) {
		e.Exe = stored
	})
}

// AddLink 新增一个外部链接，返回生成的 key；名称为空时用域名代替
func (s *Store) AddLink(name, target string) (string, error) {
	key := linkKey()
	err := s.updateEntry(key, func(e *appEntry) {
		e.Name = strings.TrimSpace(name)
		if e.Name == "" {
			e.Name = hostOf(target)
		}
		e.URL = target
	})
	return key, err
}

// UpdateLink 修改已有链接的名称与地址
func (s *Store) UpdateLink(key, name, target string) error {
	return s.updateEntry(key, func(e *appEntry) {
		if e.URL == "" {
			return
		}
		name = strings.TrimSpace(name)
		if name == "" {
			name = hostOf(target)
		}
		e.Name, e.URL = name, target
	})
}

// DeleteLink 删除链接条目；非链接条目不受影响
func (s *Store) DeleteLink(key string) error {
	return s.updateEntry(key, func(e *appEntry) {
		if e.URL != "" {
			*e = appEntry{}
		}
	})
}

// NormalizeLink 校验链接地址，缺失协议时补 https://
func NormalizeLink(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("请填写链接地址")
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("地址格式有误")
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
	default:
		return "", fmt.Errorf("只支持 http:// 或 https:// 地址")
	}
	if u.Host == "" {
		return "", fmt.Errorf("地址缺少域名")
	}
	return u.String(), nil
}

func hostOf(target string) string {
	if u, err := url.Parse(target); err == nil && u.Hostname() != "" {
		return u.Hostname()
	}
	return target
}

func linkKey() string {
	return linkPrefix + strconv.FormatInt(time.Now().UnixNano(), 36)
}

// LinkKey 判断 key 是否为链接条目
func LinkKey(key string) bool {
	return strings.HasPrefix(key, linkPrefix)
}

// updateEntry 读取集中配置、修改单个应用后写回；条目为空时删除
func (s *Store) updateEntry(key string, fn func(*appEntry)) error {
	if s.config == "" {
		return fmt.Errorf("未配置 %s 路径", ConfigFile)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	cfg, err := readConfig(s.config)
	if err != nil {
		return err
	}

	e := cfg[key]
	fn(&e)
	if entryEmpty(e) {
		delete(cfg, key)
	} else {
		cfg[key] = e
	}
	return writeConfig(s.config, cfg)
}

// entryEmpty 判断条目是否没有任何配置，用于清理空条目
func entryEmpty(e appEntry) bool {
	return e.Name == "" && e.Note == "" && !e.Pin && e.Order == 0 &&
		e.URL == "" && e.Exe == "" && len(e.Args) == 0 && len(e.Env) == 0
}
