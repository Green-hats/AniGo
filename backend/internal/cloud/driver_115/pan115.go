package driver115

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"github.com/greenhats/anigo/internal/domain"
	"github.com/greenhats/anigo/internal/util"
)

// Pan115 是 115 网盘离线下载驱动，使用浏览器 Cookie 鉴权。
// 文件下载到 115 云端；存在性检查与播放走 Web API。
type Pan115 struct {
	clientMu   sync.Mutex
	client     *http.Client
	stateMu    sync.RWMutex
	loginState domain.LoginStatus
	// reqFn 是可替换的底层请求函数（默认指向 request，测试可注入）。
	reqFn func(ctx context.Context, cfg *domain.Config, method, rawURL string, form url.Values) (map[string]interface{}, error)
}

// 115 网页端 UA。
const ua115 = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36"

// 常见请求路径（便于复用与测试）。
const (
	apiFiles       = "https://webapi.115.com/files"
	apiFileAdd     = "https://webapi.115.com/files/add"
	apiDelete      = "https://webapi.115.com/rb/delete"
	apiLixianAdd   = "https://115.com/web/lixian/?ct=lixian&ac=add_task_url"
	apiLixianList  = "https://115.com/web/lixian/?ct=lixian&ac=task_lists"
	apiFileURL     = "https://proapi.115.com/app/chrome/down"
)

// New 创建 115 驱动。
func New() domain.CloudDriver {
	return newPan115()
}

func newPan115() *Pan115 {
	p := &Pan115{}
	p.reqFn = p.request
	return p
}

// Name 返回驱动名。
func (p *Pan115) Name() string { return "115" }

func (p *Pan115) httpClient(cfg *domain.Config) *http.Client {
	p.clientMu.Lock()
	defer p.clientMu.Unlock()
	if p.client == nil {
		p.client = util.ClientFor(cfg, 20)
	}
	return p.client
}

// GetLoginStatus 返回最近登录状态。
func (p *Pan115) GetLoginStatus() domain.LoginStatus {
	p.stateMu.RLock()
	defer p.stateMu.RUnlock()
	return p.loginState
}

func (p *Pan115) setLogin(s domain.LoginStatus) {
	p.stateMu.Lock()
	p.loginState = s
	p.stateMu.Unlock()
}

// request 执行一次 115 Web API 调用，带账户 Cookie。
// 非 2xx 返回错误；JSON 解析失败也返回错误（不再静默吞掉）。
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
	req.Header.Set("Cookie", cfg.Pan115Cookie)
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	resp, err := p.httpClient(cfg).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var m map[string]interface{}
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("115 响应解析失败: %w", err)
	}
	// 115 业务错误通常带 errcode/error_msg 且 state=false
	if state, ok := m["state"].(bool); ok && !state {
		code := intVal(m["errcode"])
		msg := strVal(m["error_msg"])
		if msg == "" {
			msg = strVal(m["msg"])
		}
		if msg == "" {
			msg = "请求失败"
		}
		return m, &apiError{Code: code, Msg: msg}
	}
	return m, nil
}

// apiError 是 115 返回的业务错误。
type apiError struct {
	Code int
	Msg  string
}

func (e *apiError) Error() string { return fmt.Sprintf("115 错误(%d): %s", e.Code, e.Msg) }

// Login 验证 115 Cookie 有效性（列根目录）。
func (p *Pan115) Login(ctx context.Context, test bool, cfg *domain.Config) (bool, error) {
	if strings.TrimSpace(cfg.Pan115Cookie) == "" {
		p.setLogin(domain.LoginStatus{Configured: false, Message: "115 未配置 Cookie"})
		return false, nil
	}
	m, err := p.reqFn(ctx, cfg, "GET", apiFiles+"?aid=1&cid=0&o=user_ptime&asc=0&offset=0&show_dir=1&limit=1&show_all=0", nil)
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

// filesResponse 是 webapi.115.com/files 的响应结构。
type filesResponse struct {
	State bool `json:"state"`
	Data  []struct {
		CID  string `json:"cid"`
		FID  string `json:"fid"`
		FC   int    `json:"fc"`
		N    string `json:"n"`
		S    int64  `json:"s"`
		PC   string `json:"pc"`
	} `json:"data"`
}

// listFiles 返回目录（cid）中的条目。
func (p *Pan115) listFiles(ctx context.Context, cfg *domain.Config, cid string) ([]domain.CloudFile, error) {
	rawURL := fmt.Sprintf("%s?aid=1&cid=%s&o=user_ptime&asc=0&offset=0&show_dir=1&limit=1000&show_all=0", apiFiles, cid)
	m, err := p.reqFn(ctx, cfg, "GET", rawURL, nil)
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

// walkPath 沿路径逐层下钻。
// 返回：已到达的目录 id、路径中缺失的第一个分段及之后的分段、错误（listFiles 失败时）。
func (p *Pan115) walkPath(ctx context.Context, cfg *domain.Config, path string) (string, []string, error) {
	segs := splitPath(path)
	if len(segs) == 0 {
		return "0", nil, nil
	}
	cid := "0"
	for i, seg := range segs {
		files, err := p.listFiles(ctx, cfg, cid)
		if err != nil {
			return cid, segs[i:], err
		}
		found := ""
		for _, f := range files {
			if f.IsDir && f.Name == seg {
				found = f.ID
				break
			}
		}
		if found == "" {
			return cid, segs[i:], nil
		}
		cid = found
	}
	return cid, nil, nil
}

// ensureDir 返回路径的目录 id，必要时逐级创建。
// 单次遍历即可：已存在的部分直接下钻，缺失的部分逐级 mkdir。
func (p *Pan115) ensureDir(ctx context.Context, cfg *domain.Config, path string) string {
	segs := splitPath(path)
	if len(segs) == 0 {
		return "0"
	}
	// 沿已有路径下钻
	parent, missing, err := p.walkPath(ctx, cfg, path)
	if err != nil {
		return ""
	}
	// 逐级创建缺失部分
	for _, seg := range missing {
		id := p.mkdir(ctx, cfg, parent, seg)
		if id == "" {
			return ""
		}
		parent = id
	}
	return parent
}

// mkdir 创建文件夹并返回 id。
func (p *Pan115) mkdir(ctx context.Context, cfg *domain.Config, pid, name string) string {
	form := url.Values{}
	form.Set("pid", pid)
	form.Set("cname", name)
	m, err := p.reqFn(ctx, cfg, "POST", apiFileAdd, form)
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

// AddOfflineTask 添加磁力离线下载到 115。
// 提交成功后返回 nil；任务已存在（errcode 10008）也视为成功。
func (p *Pan115) AddOfflineTask(ctx context.Context, cfg *domain.Config, magnet, destPath string) error {
	if strings.TrimSpace(magnet) == "" {
		return errors.New("磁力链接为空")
	}
	dirID := p.ensureDir(ctx, cfg, dirOf(destPath))
	if dirID == "" {
		return fmt.Errorf("115 创建目录失败 %s", destPath)
	}
	form := url.Values{}
	form.Set("url", magnet)
	form.Set("wp_path_id", dirID)
	_, err := p.reqFn(ctx, cfg, "POST", apiLixianAdd, form)
	if err == nil {
		return nil
	}
	// 任务已存在（10008）视为成功
	var ae *apiError
	if errors.As(err, &ae) && ae.Code == 10008 {
		return nil
	}
	return err
}

// FileExists 检查云端文件是否存在。
func (p *Pan115) FileExists(ctx context.Context, cfg *domain.Config, path string) (bool, error) {
	pc, err := p.lookupPickCode(ctx, cfg, path)
	return pc != "", err
}

// FileURL 返回云端文件的可播放 URL。
func (p *Pan115) FileURL(ctx context.Context, cfg *domain.Config, path string) (string, error) {
	pc, err := p.lookupPickCode(ctx, cfg, path)
	if err != nil {
		return "", err
	}
	if pc == "" {
		return "", fmt.Errorf("云端文件不存在 %s", path)
	}
	rawURL := apiFileURL + "?pickcode=" + url.QueryEscape(pc) + "&method=get_file_url"
	m, err := p.reqFn(ctx, cfg, "GET", rawURL, nil)
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
	dirID, _, _ := p.walkPath(ctx, cfg, path)
	if dirID == "0" {
		return nil, nil
	}
	return p.listFiles(ctx, cfg, dirID)
}

// DeleteDir 递归删除云端目录。
func (p *Pan115) DeleteDir(ctx context.Context, cfg *domain.Config, path string) error {
	segs := splitPath(path)
	if len(segs) == 0 {
		return fmt.Errorf("无法删除根目录")
	}
	cid := "0"
	parent := ""
	for _, seg := range segs {
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
	_, err := p.reqFn(ctx, cfg, "POST", apiDelete, form)
	return err
}

// lookupPickCode 返回云路径的下载码（不存在时返回空串）。
func (p *Pan115) lookupPickCode(ctx context.Context, cfg *domain.Config, cloudPath string) (string, error) {
	dirID, _, _ := p.walkPath(ctx, cfg, dirOf(cloudPath))
	if dirID == "0" {
		return "", nil
	}
	files, err := p.listFiles(ctx, cfg, dirID)
	if err != nil {
		return "", err
	}
	name := baseOf(cloudPath)
	for _, f := range files {
		if !f.IsDir && f.Name == name {
			return f.PickCode, nil
		}
	}
	return "", nil
}

// 简单路径工具：父目录 / 文件名 / 分段。
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

// splitPath 将路径切分为非空分段。
func splitPath(path string) []string {
	raw := strings.Split(strings.Trim(path, "/"), "/")
	out := raw[:0]
	for _, s := range raw {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func strVal(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	if f, ok := v.(float64); ok {
		return strconv.FormatInt(int64(f), 10)
	}
	if n, ok := v.(json.Number); ok {
		return n.String()
	}
	return ""
}

func intVal(v interface{}) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case string:
		n, _ := strconv.Atoi(t)
		return n
	case json.Number:
		n, _ := t.Int64()
		return int(n)
	}
	return 0
}

func int64Val(v interface{}) int64 {
	switch t := v.(type) {
	case float64:
		return int64(t)
	case string:
		n, _ := strconv.ParseInt(t, 10, 64)
		return n
	case json.Number:
		n, _ := t.Int64()
		return n
	}
	return 0
}