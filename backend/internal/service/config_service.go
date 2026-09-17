package service

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/greenhats/anigo/internal/auth"
	"github.com/greenhats/anigo/internal/domain"
)

// ConfigService 管理应用配置与订阅列表，
// 在内存中持有它们，并通过 ConfigStore 端口持久化。
type ConfigService struct {
	mu     sync.RWMutex
	store  domain.ConfigStore
	cache  domain.Cache
	cfg    *domain.Config
	aniLst []*domain.Ani
}

// NewConfigService 从 store 加载配置与订阅。
func NewConfigService(store domain.ConfigStore, cache domain.Cache) (*ConfigService, error) {
	cfg, err := store.LoadConfig()
	if err != nil {
		return nil, err
	}
	anis, err := store.LoadAnis()
	if err != nil {
		return nil, err
	}
	return &ConfigService{
		store:  store,
		cache:  cache,
		cfg:    cfg,
		aniLst: anis,
	}, nil
}

// Get 返回当前配置快照。
func (s *ConfigService) Get() *domain.Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg.Clone()
}

// Dir 返回配置目录（经由 store）。
func (s *ConfigService) Dir() string {
	if st, ok := s.store.(interface{ Dir() string }); ok {
		return st.Dir()
	}
	return "."
}

// ConfigDirFile 返回配置目录下的绝对路径。
func (s *ConfigService) ConfigDirFile(rel string) string {
	if st, ok := s.store.(interface{ ConfigDirFile(string) string }); ok {
		return st.ConfigDirFile(rel)
	}
	return rel
}

// SetConfigRaw 将原始 JSON 体合并进配置（部分合并，
// 镜像 BeanUtil.copyProperties 的忽略空值语义）并持久化。
// 若入参携带明文密码（非空），会先转为 bcrypt 哈希再存储。
func (s *ConfigService) SetConfigRaw(raw []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur := s.cfg.Clone()
	var err error
	if raw, err = hashPasswordInRaw(raw); err != nil {
		return err
	}
	if err := mergeConfigInto(cur, raw); err != nil {
		return err
	}
	if err := s.store.SaveConfig(cur); err != nil {
		return err
	}
	s.cfg = cur
	return nil
}

// hashPasswordInRaw 将 raw 中非空的 login.password（明文）替换为 bcrypt 哈希。
func hashPasswordInRaw(raw []byte) ([]byte, error) {
	var inc struct {
		Login struct {
			Password string `json:"password"`
		} `json:"login"`
	}
	if err := json.Unmarshal(raw, &inc); err != nil {
		return nil, err
	}
	if inc.Login.Password == "" || auth.IsBcrypt(inc.Login.Password) {
		return raw, nil
	}
	hash, err := auth.HashPassword(inc.Login.Password)
	if err != nil {
		return nil, err
	}
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	if lg, ok := m["login"].(map[string]interface{}); ok {
		lg["password"] = hash
	}
	return json.Marshal(m)
}

// AniList 返回订阅列表。
func (s *ConfigService) AniList() []*domain.Ani {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneAnis(s.aniLst)
}

// SaveAniList 持久化订阅列表。
func (s *ConfigService) SaveAniList(list []*domain.Ani) error {
	return s.UpdateAniList(func(out *[]*domain.Ani) error { *out = cloneAnis(list); return nil })
}

func cloneAnis(list []*domain.Ani) []*domain.Ani {
	out := make([]*domain.Ani, len(list))
	for i, a := range list {
		out[i] = a.Clone()
	}
	return out
}

// UpdateAniList 在同一临界区完成读、改、落盘；落盘失败时内存不变。
func (s *ConfigService) UpdateAniList(update func(*[]*domain.Ani) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	list := cloneAnis(s.aniLst)
	if err := update(&list); err != nil {
		return err
	}
	if err := s.store.SaveAnis(list); err != nil {
		return err
	}
	s.aniLst = cloneAnis(list)
	return nil
}

var ErrAniNotFound = fmt.Errorf("订阅不存在")

func (s *ConfigService) UpdateAni(id string, update func(*domain.Ani) error) error {
	return s.UpdateAniList(func(list *[]*domain.Ani) error {
		for _, ani := range *list {
			if ani != nil && ani.ID == id {
				return update(ani)
			}
		}
		return ErrAniNotFound
	})
}

// ClearCache 清空内存 TTL 缓存。
func (s *ConfigService) ClearCache() {
	s.cache.Clear()
}

// ExportConfig 将配置目录打包为 zip 备份，写入 w。
// 包含 files/、torrents/ 目录以及 config.v2.json、ani.v2.json。
func (s *ConfigService) ExportConfig(w io.Writer) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	configDir := s.Dir()
	zw := zip.NewWriter(w)

	addZipDir := func(dir string) error {
		return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			if !info.Mode().IsRegular() {
				return nil
			}
			if strings.HasPrefix(filepath.Base(path), ".") {
				return nil
			}
			rel, err := filepath.Rel(configDir, path)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			fw, err := zw.Create(rel)
			if err != nil {
				return err
			}
			src, err := os.Open(path)
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(fw, src)
			closeErr := src.Close()
			return errors.Join(copyErr, closeErr)
		})
	}
	// 目录不存在时跳过（首次启动没有 files/torrents）
	for _, dir := range []string{"files", "torrents"} {
		d := filepath.Join(configDir, dir)
		if _, err := os.Stat(d); err == nil {
			if err := addZipDir(d); err != nil {
				return err
			}
		}
	}
	// 两个配置文件
	for _, name := range []string{"config.v2.json", "ani.v2.json"} {
		p := filepath.Join(configDir, name)
		if _, err := os.Stat(p); err != nil {
			return err
		}
		fw, err := zw.Create(name)
		if err != nil {
			return err
		}
		src, err := os.Open(p)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(fw, src)
		closeErr := src.Close()
		if err := errors.Join(copyErr, closeErr); err != nil {
			return err
		}
	}
	return zw.Close()
}

// ImportConfig 从 zip 备份恢复 config.v2.json 与 ani.v2.json，
// 然后重新加载内存中的配置与订阅。
const MaxBackupBytes int64 = 100 << 20

func (s *ConfigService) ImportConfig(zipPath string) error {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer zr.Close()
	if len(zr.File) > 10000 {
		return fmt.Errorf("备份文件数量过多")
	}
	files := map[string][]byte{}
	var total int64
	for _, f := range zr.File {
		name := f.Name
		if f.FileInfo().IsDir() {
			continue
		}
		if !f.Mode().IsRegular() || strings.Contains(name, "\\") || !filepath.IsLocal(name) || filepath.ToSlash(filepath.Clean(name)) != name {
			return fmt.Errorf("不安全的备份路径: %s", name)
		}
		if name != "config.v2.json" && name != "ani.v2.json" && !strings.HasPrefix(name, "files/") && !strings.HasPrefix(name, "torrents/") {
			return fmt.Errorf("未知备份文件: %s", name)
		}
		if _, exists := files[name]; exists {
			return fmt.Errorf("重复备份文件: %s", name)
		}
		if f.UncompressedSize64 > uint64(MaxBackupBytes-total) {
			return fmt.Errorf("备份解压后超过 100 MiB")
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		body, readErr := io.ReadAll(io.LimitReader(rc, MaxBackupBytes-total+1))
		closeErr := rc.Close()
		if err := errors.Join(readErr, closeErr); err != nil {
			return err
		}
		total += int64(len(body))
		if total > MaxBackupBytes {
			return fmt.Errorf("备份解压后超过 100 MiB")
		}
		files[name] = body
	}
	cfgBytes, cfgOK := files["config.v2.json"]
	aniBytes, aniOK := files["ani.v2.json"]
	if !cfgOK || !aniOK {
		return fmt.Errorf("备份必须包含 config.v2.json 和 ani.v2.json")
	}
	cfg := domain.DefaultConfig()
	var list []*domain.Ani
	if len(bytes.TrimSpace(cfgBytes)) == 0 || bytes.TrimSpace(cfgBytes)[0] != '{' {
		return fmt.Errorf("配置必须是 JSON 对象")
	}
	if err := json.Unmarshal(cfgBytes, cfg); err != nil {
		return fmt.Errorf("配置无效: %w", err)
	}
	if err := json.Unmarshal(aniBytes, &list); err != nil {
		return fmt.Errorf("订阅无效: %w", err)
	}
	if list == nil {
		return fmt.Errorf("订阅必须是 JSON 数组")
	}
	seen := map[string]bool{}
	for _, ani := range list {
		if ani == nil || strings.TrimSpace(ani.ID) == "" || seen[ani.ID] {
			return fmt.Errorf("订阅 ID 为空或重复")
		}
		seen[ani.ID] = true
		if ani.TotalEpisodeNumber < 0 || ani.TotalEpisodeNumber > 100000 {
			return fmt.Errorf("订阅总集数无效")
		}
		for _, task := range ani.DownloadTasks {
			if task.Attempts < 0 || task.Episode <= 0 || task.Hash == "" {
				return fmt.Errorf("下载记录无效")
			}
			switch task.State {
			case "pending", "submitted", "completed", "failed":
			default:
				return fmt.Errorf("下载状态无效")
			}
		}
		fillAniDefaultsFromAni(ani)
	}
	if cfg.UUID == "" {
		cfg.UUID = domain.NewUUID()
	}
	if cfg.Exclude == nil {
		cfg.Exclude = []string{}
	}
	if cfg.NotificationConfigList == nil {
		cfg.NotificationConfigList = []domain.NotificationConfig{}
	}
	if cfg.ReverseProxyTrustIpList == nil {
		cfg.ReverseProxyTrustIpList = []string{}
	}
	files["config.v2.json"], err = json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	files["ani.v2.json"], err = json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	restorer, ok := s.store.(interface{ RestoreBackup(map[string][]byte) error })
	if !ok {
		return fmt.Errorf("当前存储不支持事务恢复")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := restorer.RestoreBackup(files); err != nil {
		return err
	}
	s.cfg, s.aniLst = cfg, cloneAnis(list)
	s.cache.Clear()
	return nil
}

// mergeConfigInto 将 raw 中 JSON 存在的字段合并进 cur，
// 保留空登录凭据与 UUID。
func mergeConfigInto(cur *domain.Config, raw []byte) error {
	curBytes, err := json.Marshal(cur)
	if err != nil {
		return err
	}
	curMap := map[string]interface{}{}
	if err := json.Unmarshal(curBytes, &curMap); err != nil {
		return err
	}
	incoming := map[string]interface{}{}
	if err := json.Unmarshal(raw, &incoming); err != nil {
		return err
	}
	for k, v := range incoming {
		if k == "gitInfo" {
			continue
		}
		curMap[k] = v
	}
	mergedBytes, err := json.Marshal(curMap)
	if err != nil {
		return err
	}
	merged := &domain.Config{}
	if err := json.Unmarshal(mergedBytes, merged); err != nil {
		return err
	}
	if merged.Login.Username == "" {
		merged.Login.Username = cur.Login.Username
	}
	if merged.Login.Password == "" {
		merged.Login.Password = cur.Login.Password
	}
	merged.UUID = cur.UUID
	merged.GitInfo = cur.GitInfo
	*cur = *merged
	return nil
}
