package driver115

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/greenhats/anigo/internal/domain"
	"github.com/greenhats/anigo/internal/util"
)

// Pan115 是 115 网盘离线下载驱动，使用浏览器 Cookie 鉴权。
// 文件下载到 115 云端；存在性检查与播放走 Web API。
type Pan115 struct {
	mu         sync.Mutex
	client     *http.Client
	loginState domain.LoginStatus
}

// 115 网页端 UA。
const ua115 = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36"

// New 创建 115 驱动。
func New() domain.CloudDriver { return &Pan115{} }

// Name 返回驱动名。
func (p *Pan115) Name() string { return "115" }

func (p *Pan115) httpClient(cfg *domain.Config) *http.Client {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.client == nil {
		p.client = util.ClientFor(cfg, 20)
	}
	return p.client
}

func (p *Pan115) cookie(cfg *domain.Config) string {
	return cfg.Pan115Cookie
}

// request 执行一次 115 Web API 调用，带账户 Cookie。
func (p *Pan115) request(ctx context.Context, cfg *domain.Config, method, rawURL string, form url.Values) (map[string]interface{}, error) {
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", ua115)
	req.Header.Set("Cookie", p.cookie(cfg))
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	resp, err := p.httpClient(cfg).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	var m map[string]interface{}
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// Login 验证 115 Cookie 有效性（列根目录）。
func (p *Pan115) Login(ctx context.Context, test bool, cfg *domain.Config) (bool, error) {
	if strings.TrimSpace(cfg.Pan115Cookie) == "" {
		p.setLogin(domain.LoginStatus{Configured: false, Message: "115 未配置 Cookie"})
		return false, nil
	}
	m, err := p.request(ctx, cfg, "GET", "https://webapi.115.com/files?aid=1&cid=0&o=user_ptime&asc=0&offset=0&show_dir=1&limit=1&show_all=0", nil)
	if err != nil {
		p.setLogin(domain.LoginStatus{Configured: true, Message: "115 登录失败: " + err.Error()})
		return false, err
	}
	state, _ := m["state"].(bool)
	if !state {
		p.setLogin(domain.LoginStatus{Configured: true, Message: "115 Cookie 无效或已过期"})
		return false, nil
	}
	p.setLogin(domain.LoginStatus{Configured: true, OK: true})
	return true, nil
}

// GetLoginStatus 返回最近登录状态。
func (p *Pan115) GetLoginStatus() domain.LoginStatus {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.loginState
}

func (p *Pan115) setLogin(s domain.LoginStatus) {
	p.mu.Lock()
	p.loginState = s
	p.mu.Unlock()
}

// listFiles 返回目录（cid）中的条目。
func (p *Pan115) listFiles(ctx context.Context, cfg *domain.Config, cid string) ([]domain.CloudFile, error) {
	rawURL := fmt.Sprintf("https://webapi.115.com/files?aid=1&cid=%s&o=user_ptime&asc=0&offset=0&show_dir=1&limit=1000&show_all=0", cid)
	m, err := p.request(ctx, cfg, "GET", rawURL, nil)
	if err != nil {
		return nil, err
	}
	data, _ := m["data"].([]interface{})
	var out []domain.CloudFile
	for _, item := range data {
		im, _ := item.(map[string]interface{})
		cloudID := strVal(im["cid"])
		fc, _ := im["fc"].(float64)
		isDir := fc == 0
		if !isDir {
			cloudID = strVal(im["fid"])
		}
		out = append(out, domain.CloudFile{
			Name:     strVal(im["n"]),
			Size:     int64Val(im["s"]),
			IsDir:    isDir,
			ID:       cloudID,
			PickCode: strVal(im["pc"]),
		})
	}
	return out, nil
}

// findDirID 定位云路径对应的目录 id（不存在时返回空串）。
func (p *Pan115) findDirID(ctx context.Context, cfg *domain.Config, path string) string {
	cid := "0"
	for _, seg := range strings.Split(strings.Trim(path, "/"), "/") {
		if seg == "" {
			continue
		}
		files, err := p.listFiles(ctx, cfg, cid)
		if err != nil {
			return ""
		}
		found := ""
		for _, f := range files {
			if f.IsDir && f.Name == seg {
				found = f.ID
				break
			}
		}
		if found == "" {
			return ""
		}
		cid = found
	}
	return cid
}

// mkdir 创建文件夹并返回 id。
func (p *Pan115) mkdir(ctx context.Context, cfg *domain.Config, pid, name string) string {
	form := url.Values{}
	form.Set("pid", pid)
	form.Set("cname", name)
	m, err := p.request(ctx, cfg, "POST", "https://webapi.115.com/files/add", form)
	if err != nil {
		return ""
	}
	if id, ok := m["cid"].(string); ok {
		return id
	}
	if data, ok := m["data"].(map[string]interface{}); ok {
		return strVal(data["cid"])
	}
	return ""
}

// ensureDir 返回路径的目录 id，必要时逐级创建。
func (p *Pan115) ensureDir(ctx context.Context, cfg *domain.Config, path string) string {
	path = strings.Trim(path, "/")
	if path == "" {
		return "0"
	}
	if id := p.findDirID(ctx, cfg, path); id != "" {
		return id
	}
	parent := "0"
	prefix := ""
	for _, seg := range strings.Split(path, "/") {
		if seg == "" {
			continue
		}
		if prefix == "" {
			prefix = seg
		} else {
			prefix = prefix + "/" + seg
		}
		id := p.findDirID(ctx, cfg, prefix)
		if id == "" {
			id = p.mkdir(ctx, cfg, parent, seg)
		}
		parent = id
	}
	return parent
}

// AddOfflineTask 添加磁力离线下载到 115 并等待完成。
func (p *Pan115) AddOfflineTask(ctx context.Context, cfg *domain.Config, magnet, destPath string) error {
	if strings.TrimSpace(magnet) == "" {
		return fmt.Errorf("磁力链接为空")
	}
	dirID := p.ensureDir(ctx, cfg, dirOf(destPath))
	if dirID == "" {
		return fmt.Errorf("115 创建目录失败 %s", destPath)
	}
	form := url.Values{}
	form.Set("url", magnet)
	form.Set("wp_path_id", dirID)
	m, err := p.request(ctx, cfg, "POST", "https://115.com/web/lixian/?ct=lixian&ac=add_task_url", form)
	if err != nil {
		return fmt.Errorf("115 添加离线下载失败: %w", err)
	}
	state, _ := m["state"].(bool)
	if !state {
		// errcode 10008 = 任务已存在 → 视为已添加，继续等待
		errcode := int64Val(m["errcode"])
		if errcode != 10008 {
			return fmt.Errorf("115 添加离线下载失败: %v", m["error_msg"])
		}
	}
	return nil
}

// FileExists 检查云端文件是否存在。
func (p *Pan115) FileExists(ctx context.Context, cfg *domain.Config, path string) (bool, error) {
	pc := p.lookupPickCode(ctx, cfg, path)
	return pc != "", nil
}

// FileURL 返回云端文件的可播放 URL。
func (p *Pan115) FileURL(ctx context.Context, cfg *domain.Config, path string) (string, error) {
	pc := p.lookupPickCode(ctx, cfg, path)
	if pc == "" {
		return "", fmt.Errorf("云端文件不存在 %s", path)
	}
	rawURL := "https://proapi.115.com/app/chrome/down?pickcode=" + url.QueryEscape(pc) + "&method=get_file_url"
	m, err := p.request(ctx, cfg, "GET", rawURL, nil)
	if err != nil {
		return "", err
	}
	data, ok := m["data"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("115 获取文件 URL 失败")
	}
	for _, v := range data {
		fm, ok := v.(map[string]interface{})
		if !ok {
			continue
		}
		um, ok := fm["url"].(map[string]interface{})
		if !ok {
			continue
		}
		if u := strVal(um["url"]); u != "" {
			return u, nil
		}
	}
	return "", fmt.Errorf("115 未返回播放 URL")
}

// ListDir 列出云端目录的文件。
func (p *Pan115) ListDir(ctx context.Context, cfg *domain.Config, path string) ([]domain.CloudFile, error) {
	dirID := p.findDirID(ctx, cfg, path)
	if dirID == "" {
		return nil, nil
	}
	return p.listFiles(ctx, cfg, dirID)
}

// DeleteDir 递归删除云端目录。
func (p *Pan115) DeleteDir(ctx context.Context, cfg *domain.Config, path string) error {
	cid := "0"
	parent := ""
	for _, seg := range strings.Split(strings.Trim(path, "/"), "/") {
		if seg == "" {
			continue
		}
		files, err := p.listFiles(ctx, cfg, cid)
		if err != nil {
			return err
		}
		found := ""
		for _, f := range files {
			if f.IsDir && f.Name == seg {
				found = f.ID
				break
			}
		}
		if found == "" {
			return fmt.Errorf("目录不存在 %s", path)
		}
		parent = cid
		cid = found
	}
	if cid == "0" || parent == "" {
		return fmt.Errorf("无法删除根目录")
	}
	form := url.Values{}
	form.Set("pid", parent)
	form.Set("fid[0]", cid)
	m, err := p.request(ctx, cfg, "POST", "https://webapi.115.com/rb/delete", form)
	if err != nil {
		return err
	}
	state, _ := m["state"].(bool)
	if !state {
		return fmt.Errorf("115 删除目录失败")
	}
	return nil
}

// lookupPickCode 返回云路径的下载码（不存在时返回空串）。
func (p *Pan115) lookupPickCode(ctx context.Context, cfg *domain.Config, cloudPath string) string {
	dirID := p.findDirID(ctx, cfg, dirOf(cloudPath))
	if dirID == "" {
		return ""
	}
	files, err := p.listFiles(ctx, cfg, dirID)
	if err != nil {
		return ""
	}
	name := baseOf(cloudPath)
	for _, f := range files {
		if !f.IsDir && f.Name == name {
			return f.PickCode
		}
	}
	return ""
}

// 简单路径工具：父目录 / 文件名。
func dirOf(path string) string {
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[:i]
	}
	return ""
}

func baseOf(path string) string {
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[i+1:]
	}
	return path
}

func strVal(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	if f, ok := v.(float64); ok {
		return fmt.Sprintf("%d", int64(f))
	}
	return ""
}

func int64Val(v interface{}) int64 {
	switch t := v.(type) {
	case float64:
		return int64(t)
	case string:
		var n int64
		for _, c := range t {
			if c < '0' || c > '9' {
				return n
			}
			n = n*10 + int64(c-'0')
		}
		return n
	}
	return 0
}