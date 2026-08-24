package service

import (
	"encoding/json"
	"sync"

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
	return s.cfg
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
func (s *ConfigService) SetConfigRaw(raw []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur := s.cfg
	if err := mergeConfigInto(cur, raw); err != nil {
		return err
	}
	return s.store.SaveConfig(cur)
}

// Sync 持久化当前配置。
func (s *ConfigService) Sync() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.store.SaveConfig(s.cfg)
}

// AniList 返回订阅列表。
func (s *ConfigService) AniList() []*domain.Ani {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.aniLst
}

// SaveAniList 持久化订阅列表。
func (s *ConfigService) SaveAniList(list []*domain.Ani) error {
	s.mu.Lock()
	s.aniLst = list
	s.mu.Unlock()
	return s.store.SaveAnis(list)
}

// ClearCache 清空内存 TTL 缓存。
func (s *ConfigService) ClearCache() {
	s.cache.Clear()
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