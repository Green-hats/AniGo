package driver115

import (
	"context"
	"encoding/json"
	"net/url"
	"testing"

	"github.com/greenhats/anigo/internal/domain"
)

func TestSplitPath(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"/", nil},
		{"番剧", []string{"番剧"}},
		{"番剧/间谍过家家/Season 1", []string{"番剧", "间谍过家家", "Season 1"}},
		{"/番剧//间谍过家家/", []string{"番剧", "间谍过家家"}},
	}
	for _, c := range cases {
		got := splitPath(c.in)
		if len(got) != len(c.want) {
			t.Errorf("splitPath(%q) = %v, want %v", c.in, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("splitPath(%q) = %v, want %v", c.in, got, c.want)
				break
			}
		}
	}
}

func TestPathHelpers(t *testing.T) {
	if dirOf("番剧/间谍过家家/Season 1") != "番剧/间谍过家家" {
		t.Error("dirOf wrong")
	}
	if baseOf("番剧/间谍过家家/Season 1") != "Season 1" {
		t.Error("baseOf wrong")
	}
	if baseOf("file") != "file" {
		t.Error("baseOf single wrong")
	}
}

func TestStrValVariants(t *testing.T) {
	if strVal("abc") != "abc" {
		t.Error("string")
	}
	if strVal(float64(123)) != "123" {
		t.Error("float64")
	}
	if strVal(json.Number("456")) != "456" {
		t.Error("json.Number")
	}
	if strVal(nil) != "" {
		t.Error("nil")
	}
	if int64Val(float64(99)) != 99 {
		t.Error("int64 float")
	}
	if int64Val("42") != 42 {
		t.Error("int64 string")
	}
}

// fakeTree 是一个内存目录树，模拟 115 的 cid -> 子目录结构。
type fakeTree struct {
	tree map[string]map[string]string // cid -> name->childCID
	files map[string][]domain.CloudFile // cid -> 文件
	mk   map[string]string              // "pid/name" -> new cid
	next int
}

func newFakeTree() *fakeTree {
	return &fakeTree{
		tree: map[string]map[string]string{
			"0":   {"番剧": "100"},
			"100": {"间谍过家家": "200"},
			"200": {"Season 1": "201"},
		},
		files: map[string][]domain.CloudFile{
			"201": {{Name: "ep.mkv", Size: 1024, PickCode: "PICK1"}},
		},
		mk: map[string]string{},
	}
}

// reqFn 构造一个模拟 115 API 的请求函数。
func (f *fakeTree) reqFn(ctx context.Context, cfg *domain.Config, method, rawURL string, form url.Values) (map[string]interface{}, error) {
	u, _ := url.Parse(rawURL)
	switch u.Path {
	case "/files":
		cid := u.Query().Get("cid")
		if cid == "" {
			cid = "0"
		}
		var data []interface{}
		for name, child := range f.tree[cid] {
			data = append(data, map[string]interface{}{
				"cid": child, "fc": float64(0), "n": name, "s": 0, "pc": "",
			})
		}
		for _, fl := range f.files[cid] {
			data = append(data, map[string]interface{}{
				"fid": "x" + fl.Name, "fc": float64(1), "n": fl.Name, "s": fl.Size, "pc": fl.PickCode,
			})
		}
		return map[string]interface{}{"state": true, "data": data}, nil
	case "/files/add":
		pid := form.Get("pid")
		name := form.Get("cname")
		cid := "NEW-" + string(rune('0'+f.next))
		f.next++
		f.mk[pid+"/"+name] = cid
		if f.tree[pid] == nil {
			f.tree[pid] = map[string]string{}
		}
		f.tree[pid][name] = cid
		f.tree[cid] = map[string]string{}
		return map[string]interface{}{"state": true, "cid": cid}, nil
	case "/web/lixian/":
		return map[string]interface{}{"state": true}, nil
	case "/rb/delete":
		return map[string]interface{}{"state": true}, nil
	case "/app/chrome/down":
		return map[string]interface{}{
			"state": true,
			"data": map[string]interface{}{
				"video": map[string]interface{}{
					"url": map[string]interface{}{"url": "https://cdn.example.com/file"},
				},
			},
		}, nil
	default:
		return map[string]interface{}{"state": false, "errcode": 404, "error_msg": "no such"}, nil
	}
}

func TestEnsureDirExistingPath(t *testing.T) {
	f := newFakeTree()
	p := newPan115()
	p.reqFn = f.reqFn
	cfg := &domain.Config{Pan115Cookie: "t"}
	got := p.ensureDir(context.Background(), cfg, "番剧/间谍过家家/Season 1")
	if got != "201" {
		t.Fatalf("已存在路径应返回既有 id 201, got %s", got)
	}
	if len(f.mk) != 0 {
		t.Errorf("已存在路径不应触发 mkdir, got %v", f.mk)
	}
}

func TestEnsureDirCreatesMissingTail(t *testing.T) {
	f := newFakeTree()
	p := newPan115()
	p.reqFn = f.reqFn
	cfg := &domain.Config{Pan115Cookie: "t"}
	got := p.ensureDir(context.Background(), cfg, "番剧/间谍过家家/Season 2")
	if got != "NEW-0" {
		t.Fatalf("应创建 Season 2 并返回新 id NEW-0, got %s", got)
	}
	if f.mk["200/Season 2"] != "NEW-0" {
		t.Errorf("应调用 mkdir(200, Season 2), got %v", f.mk)
	}
}

func TestEnsureDirCreatesWholeChain(t *testing.T) {
	f := newFakeTree()
	p := newPan115()
	p.reqFn = f.reqFn
	cfg := &domain.Config{Pan115Cookie: "t"}
	got := p.ensureDir(context.Background(), cfg, "剧场版/某剧场/子目录")
	if got != "NEW-2" {
		t.Fatalf("应逐级创建, got %s", got)
	}
	if f.mk["0/剧场版"] != "NEW-0" || f.mk["NEW-0/某剧场"] != "NEW-1" || f.mk["NEW-1/子目录"] != "NEW-2" {
		t.Errorf("创建链错误: %v", f.mk)
	}
}

func TestEnsureDirEmpty(t *testing.T) {
	f := newFakeTree()
	p := newPan115()
	p.reqFn = f.reqFn
	cfg := &domain.Config{Pan115Cookie: "t"}
	if got := p.ensureDir(context.Background(), cfg, ""); got != "0" {
		t.Fatalf("空路径应返回 0, got %s", got)
	}
}

// AddOfflineTask 对 errcode 10008（任务已存在）应视为成功。
func TestAddOfflineTaskTaskExistsIsOK(t *testing.T) {
	p := newPan115()
	p.reqFn = func(ctx context.Context, cfg *domain.Config, method, rawURL string, form url.Values) (map[string]interface{}, error) {
		u, _ := url.Parse(rawURL)
		switch u.Path {
		case "/files":
			return map[string]interface{}{"state": true, "data": []interface{}{}}, nil
		case "/files/add":
			return map[string]interface{}{"state": true, "cid": "NEW0"}, nil
		default:
			// 模拟 lixian 返回 10008 任务已存在 → 通过 apiError 返回
			return nil, &apiError{Code: 10008, Msg: "任务已存在"}
		}
	}
	cfg := &domain.Config{Pan115Cookie: "t"}
	err := p.AddOfflineTask(context.Background(), cfg, "magnet:?xt=urn:btih:abc", "番剧/某番/Season 1/file.mkv")
	if err != nil {
		t.Fatalf("任务已存在(10008)应视为成功, got %v", err)
	}
}

// AddOfflineTask 其他错误（非 10008）应返回错误。
func TestAddOfflineTaskRealError(t *testing.T) {
	p := newPan115()
	p.reqFn = func(ctx context.Context, cfg *domain.Config, method, rawURL string, form url.Values) (map[string]interface{}, error) {
		u, _ := url.Parse(rawURL)
		switch u.Path {
		case "/files":
			return map[string]interface{}{"state": true, "data": []interface{}{}}, nil
		case "/files/add":
			return map[string]interface{}{"state": true, "cid": "NEW0"}, nil
		default:
			return nil, &apiError{Code: 500, Msg: "服务繁忙"}
		}
	}
	cfg := &domain.Config{Pan115Cookie: "t"}
	err := p.AddOfflineTask(context.Background(), cfg, "magnet:?xt=urn:btih:abc", "番剧/某番/Season 1/file.mkv")
	if err == nil {
		t.Fatal("非 10008 错误应返回错误")
	}
}

// FileExists 应通过查找 pickcode 判断文件存在。
func TestFileExists(t *testing.T) {
	f := newFakeTree()
	p := newPan115()
	p.reqFn = f.reqFn
	cfg := &domain.Config{Pan115Cookie: "t"}
	exists, err := p.FileExists(context.Background(), cfg, "番剧/间谍过家家/Season 1/ep.mkv")
	if err != nil {
		t.Fatalf("FileExists err: %v", err)
	}
	if !exists {
		t.Fatal("文件应存在")
	}
	missing, _ := p.FileExists(context.Background(), cfg, "番剧/间谍过家家/Season 1/不存在.mkv")
	if missing {
		t.Fatal("不存在文件不应报告存在")
	}
}

// FileURL 应返回可播放 URL。
func TestFileURL(t *testing.T) {
	f := newFakeTree()
	p := newPan115()
	p.reqFn = f.reqFn
	cfg := &domain.Config{Pan115Cookie: "t"}
	u, err := p.FileURL(context.Background(), cfg, "番剧/间谍过家家/Season 1/ep.mkv")
	if err != nil {
		t.Fatalf("FileURL err: %v", err)
	}
	if u != "https://cdn.example.com/file" {
		t.Fatalf("FileURL = %q", u)
	}
}

// 登录成功路径。
func TestLoginSuccess(t *testing.T) {
	p := newPan115()
	p.reqFn = func(ctx context.Context, cfg *domain.Config, method, rawURL string, form url.Values) (map[string]interface{}, error) {
		return map[string]interface{}{"state": true, "data": []interface{}{}}, nil
	}
	cfg := &domain.Config{Pan115Cookie: "t"}
	ok, err := p.Login(context.Background(), true, cfg)
	if err != nil {
		t.Fatalf("Login err: %v", err)
	}
	if !ok {
		t.Fatal("Login 应成功")
	}
	st := p.GetLoginStatus()
	if !st.OK {
		t.Fatalf("登录状态未更新: %+v", st)
	}
}

// 未配置 Cookie 时 Login 应返回未配置状态。
func TestLoginNoCookie(t *testing.T) {
	p := newPan115()
	p.reqFn = func(ctx context.Context, cfg *domain.Config, method, rawURL string, form url.Values) (map[string]interface{}, error) {
		return nil, nil
	}
	cfg := &domain.Config{}
	ok, err := p.Login(context.Background(), true, cfg)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if ok {
		t.Fatal("未配置 Cookie 不应登录成功")
	}
	st := p.GetLoginStatus()
	if st.Configured {
		t.Fatalf("未配置 Cookie 状态应 configured=false: %+v", st)
	}
}