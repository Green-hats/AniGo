package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/greenhats/anigo/internal/domain"
)

func TestNewJSONStorePaths(t *testing.T) {
	dir := t.TempDir()
	s := NewJSONStore(dir)
	if got := s.Dir(); got != dir {
		t.Errorf("Dir() = %q, want %q", got, dir)
	}
	if got := s.ConfigDirFile("sub/x"); got != filepath.Join(dir, "sub/x") {
		t.Errorf("ConfigDirFile() = %q", got)
	}
	if s.configPath != filepath.Join(dir, ConfigFile) {
		t.Errorf("configPath = %q", s.configPath)
	}
	if s.aniPath != filepath.Join(dir, AniFile) {
		t.Errorf("aniPath = %q", s.aniPath)
	}
}

func TestLoadConfigCreatesDefault(t *testing.T) {
	s := NewJSONStore(t.TempDir())
	c, err := s.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	def := domain.DefaultConfig()
	if c.MikanHost != def.MikanHost {
		t.Errorf("MikanHost = %q, want default %q", c.MikanHost, def.MikanHost)
	}
	if c.UUID == "" {
		t.Error("UUID 应为非空")
	}
	if _, err := os.Stat(s.configPath); err != nil {
		t.Errorf("配置文件未创建: %v", err)
	}
}

func TestSaveLoadConfigRoundTrip(t *testing.T) {
	s := NewJSONStore(t.TempDir())
	want := &domain.Config{MikanHost: "https://example.com", RssSleepMinutes: 5, Exclude: []string{"x"}, Login: domain.Login{Username: "u", Password: "p"}}
	if err := s.SaveConfig(want); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	got, err := s.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if got.MikanHost != want.MikanHost || got.RssSleepMinutes != want.RssSleepMinutes {
		t.Errorf("读回不一致: %+v", got)
	}
	if len(got.Exclude) != 1 || got.Exclude[0] != "x" {
		t.Errorf("Exclude = %v", got.Exclude)
	}
	if got.Login.Username != "u" {
		t.Errorf("Login.Username = %q", got.Login.Username)
	}
}

func TestLoadConfigFillsMissingFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ConfigFile)
	// 写入一个只含部分字段的文件
	partial := map[string]interface{}{
		"mikanHost": "https://partial.example",
		"rssSleepMinutes": 30,
	}
	b, _ := json.Marshal(partial)
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	s := NewJSONStore(dir)
	c, err := s.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if c.MikanHost != "https://partial.example" {
		t.Errorf("保留已有字段失败: %q", c.MikanHost)
	}
	def := domain.DefaultConfig()
	if c.RssSleepMinutes != 30 {
		t.Errorf("RssSleepMinutes = %d, want 30", c.RssSleepMinutes)
	}
	if c.TmdbApi != def.TmdbApi {
		t.Errorf("缺失字段未填默认值: TmdbApi = %q, want %q", c.TmdbApi, def.TmdbApi)
	}
	if c.Exclude == nil {
		t.Error("Exclude 应为空切片而非 nil")
	}
	if c.NotificationConfigList == nil {
		t.Error("NotificationConfigList 应为空切片而非 nil")
	}
}

func TestLoadAnisCreatesEmptyList(t *testing.T) {
	s := NewJSONStore(t.TempDir())
	list, err := s.LoadAnis()
	if err != nil {
		t.Fatalf("LoadAnis: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("list = %v, want empty", list)
	}
	if _, err := os.Stat(s.aniPath); err != nil {
		t.Errorf("ani 文件未创建: %v", err)
	}
}

func TestSaveLoadAnisRoundTrip(t *testing.T) {
	s := NewJSONStore(t.TempDir())
	want := []*domain.Ani{
		{ID: "a1", Title: "番剧A", Match: []string{}},
		{ID: "a2", Title: "番剧B", Exclude: []string{"720P"}},
	}
	if err := s.SaveAnis(want); err != nil {
		t.Fatalf("SaveAnis: %v", err)
	}
	got, err := s.LoadAnis()
	if err != nil {
		t.Fatalf("LoadAnis: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].ID != "a1" || got[0].Title != "番剧A" {
		t.Errorf("got[0] = %+v", got[0])
	}
	// 缺失字段应被填充默认值
	if len(got[0].Exclude) != 4 {
		t.Errorf("Exclude 默认值未填充: %v", got[0].Exclude)
	}
	if got[0].CustomEpisodeGroupIndex != 2 {
		t.Errorf("CustomEpisodeGroupIndex = %d, want 2", got[0].CustomEpisodeGroupIndex)
	}
	if got[0].CustomRenameTemplate == "" {
		t.Error("CustomRenameTemplate 应填充默认值")
	}
}

func TestFillAniDefaults(t *testing.T) {
	a := &domain.Ani{}
	fillAniDefaults(a)
	if len(a.Exclude) == 0 {
		t.Error("Exclude 应填充默认")
	}
	if a.CustomEpisodeStr == "" {
		t.Error("CustomEpisodeStr 应填充默认")
	}
	if a.StandbyRssList == nil || a.Match == nil || a.NotDownload == nil {
		t.Error("切片字段应为空切片而非 nil")
	}
}

func TestLoadConfigKeepsNilSlicesEmpty(t *testing.T) {
	// 旧文件可能序列化了 null，加载后应为 [] 而非 nil
	s := NewJSONStore(t.TempDir())
	c := &domain.Config{MikanHost: "h"}
	if err := s.SaveConfig(c); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	// 手工把 exclude 改成 null 再加载
	path := s.configPath
	var raw map[string]json.RawMessage
	b, _ := os.ReadFile(path)
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatal(err)
	}
	raw["exclude"] = json.RawMessage("null")
	raw["notificationConfigList"] = json.RawMessage("null")
	nb, _ := json.Marshal(raw)
	if err := os.WriteFile(path, nb, 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := s.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if got.Exclude == nil {
		t.Error("Exclude 应为非 nil")
	}
	if got.NotificationConfigList == nil {
		t.Error("NotificationConfigList 应为非 nil")
	}
}