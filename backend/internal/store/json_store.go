package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"

	"github.com/greenhats/anigo/internal/domain"
)

// 文件名，与遗留配置布局保持一致。
const (
	ConfigFile = "config.v2.json"
	AniFile    = "ani.v2.json"
)

// JSONStore 以 JSON 文件持久化配置与订阅列表。
type JSONStore struct {
	mu         sync.RWMutex
	dir        string
	configPath string
	aniPath    string
}

// NewJSONStore 创建以 dir（绝对路径）为根目录的存储。
func NewJSONStore(dir string) *JSONStore {
	return &JSONStore{
		dir:        dir,
		configPath: filepath.Join(dir, ConfigFile),
		aniPath:    filepath.Join(dir, AniFile),
	}
}

// Dir 返回配置目录（绝对路径）。
func (s *JSONStore) Dir() string { return s.dir }

// ConfigDirFile 返回配置目录下的绝对路径。
func (s *JSONStore) ConfigDirFile(rel string) string { return filepath.Join(s.dir, rel) }

// LoadConfig 读取配置文件，缺失时创建默认配置。
func (s *JSONStore) LoadConfig() (*domain.Config, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	def := domain.DefaultConfig()
	if def.UUID == "" {
		def.UUID = domain.NewUUID()
	}
	if _, err := os.Stat(s.configPath); os.IsNotExist(err) {
		if err := s.writeJSONLocked(s.configPath, def); err != nil {
			return nil, err
		}
	}
	c := &domain.Config{}
	if err := s.readJSON(s.configPath, c); err != nil {
		return nil, err
	}
	// 只对文件中缺失的字段应用默认值
	present := map[string]bool{}
	if raw, err := os.ReadFile(s.configPath); err == nil {
		var rm map[string]json.RawMessage
		if json.Unmarshal(raw, &rm) == nil {
			for k := range rm {
				present[k] = true
			}
		}
	}
	fillConfigDefaults(c, def, present)
	if c.UUID == "" {
		c.UUID = domain.NewUUID()
	}
	return c, nil
}

// SaveConfig 持久化配置。
func (s *JSONStore) SaveConfig(c *domain.Config) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writeJSONLocked(s.configPath, c)
}

// LoadAnis 读取订阅列表。
func (s *JSONStore) LoadAnis() ([]*domain.Ani, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var list []*domain.Ani
	if _, err := os.Stat(s.aniPath); os.IsNotExist(err) {
		list = []*domain.Ani{}
		if err := s.writeJSONLocked(s.aniPath, list); err != nil {
			return nil, err
		}
		return list, nil
	}
	if err := s.readJSON(s.aniPath, &list); err != nil {
		return nil, err
	}
	for _, a := range list {
		if a != nil {
			fillAniDefaults(a)
		}
	}
	return list, nil
}

// SaveAnis 持久化订阅列表。
func (s *JSONStore) SaveAnis(list []*domain.Ani) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writeJSONLocked(s.aniPath, list)
}

func (s *JSONStore) readJSON(path string, v interface{}) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

// writeJSONLocked 以原子方式写入美化后的 JSON（先写临时文件再重命名）。
func (s *JSONStore) writeJSONLocked(path string, v interface{}) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// fillConfigDefaults 将 def 中"文件中缺失"的字段应用到 c。
// 通过反射遍历 Config 的 json tag 与 present 对比，避免逐字段手写（新增字段自动获得默认值）。
func fillConfigDefaults(c, def *domain.Config, present map[string]bool) {
	cv := reflect.ValueOf(c).Elem()
	dv := reflect.ValueOf(def).Elem()
	t := cv.Type()
	for i := 0; i < t.NumField(); i++ {
		key := jsonTagName(t.Field(i))
		if key == "" || present[key] {
			continue
		}
		cv.Field(i).Set(dv.Field(i))
	}
	// 列表始终序列化为 []（前端依赖 .length/.push）
	if c.Exclude == nil {
		c.Exclude = []string{}
	}
	if c.NotificationConfigList == nil {
		c.NotificationConfigList = []domain.NotificationConfig{}
	}
	if c.ReverseProxyTrustIpList == nil {
		c.ReverseProxyTrustIpList = []string{}
	}
}

// jsonTagName 返回字段 json tag 的键名（去 omitempty 等选项，无 tag 返回空）。
func jsonTagName(f reflect.StructField) string {
	key := f.Tag.Get("json")
	if key == "" {
		return ""
	}
	if i := strings.IndexByte(key, ','); i >= 0 {
		key = key[:i]
	}
	return key
}

func fillAniDefaults(a *domain.Ani) {
	if len(a.Exclude) == 0 {
		a.Exclude = []string{"720[Pp]", "\\d-\\d", "合集", "特别篇"}
	}
	if a.CustomEpisodeStr == "" {
		a.CustomEpisodeStr = domain.RENAME_REG_STR()
	}
	if a.CustomEpisodeGroupIndex == 0 {
		a.CustomEpisodeGroupIndex = 2
	}
	if a.CustomRenameTemplate == "" {
		a.CustomRenameTemplate = "[${subgroup}] ${title} S${seasonFormat}E${episodeFormat}"
	}
	if a.StandbyRssList == nil {
		a.StandbyRssList = []domain.StandbyRss{}
	}
	if a.Match == nil {
		a.Match = []string{}
	}
	if a.NotDownload == nil {
		a.NotDownload = []float64{}
	}
	if a.CustomPriorityKeywords == nil {
		a.CustomPriorityKeywords = []string{}
	}
	if a.CustomTags == nil {
		a.CustomTags = []string{}
	}
}