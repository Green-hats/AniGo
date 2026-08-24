package rename

import (
	"testing"

	"github.com/greenhats/anigo/internal/domain"
)

func testConfig() *domain.Config {
	return &domain.Config{Skip5: true}
}

// 回归: RenameWithEpisode（AI 解析路径）应按给定集号渲染重命名模板。
func TestRenameWithEpisodeFormats(t *testing.T) {
	cfg := testConfig()
	ani := &domain.Ani{Season: 1, Offset: 0, Ova: false, Title: "测试番剧", Subgroup: "TSDM字幕组"}
	cases := []struct {
		ep   float64
		want string
	}{
		{4, "测试番剧 S01E04"},
		{12, "测试番剧 S01E12"},
	}
	for _, c := range cases {
		it := &domain.Item{Title: "dummy", Subgroup: "TSDM字幕组", Torrent: "magnet:?xt=urn:btih:abc"}
		if !RenameWithEpisode(ani, it, cfg, c.ep) {
			t.Errorf("ep=%v: RenameWithEpisode 应成功", c.ep)
			continue
		}
		if it.ReName != c.want {
			t.Errorf("ep=%v: ReName = %q, want %q", c.ep, it.ReName, c.want)
		}
		if it.Episode != c.ep {
			t.Errorf("ep=%v: Episode = %v", c.ep, it.Episode)
		}
	}
}

// 回归: Offset 偏移应叠加到外部给定的集数上(用于补集数从1开始但源从N开始的情况)。
func TestRenameWithEpisodeOffsetApplied(t *testing.T) {
	cfg := testConfig()
	ani := &domain.Ani{Season: 1, Offset: -1, Ova: false, Title: "测试番剧", Subgroup: "TSDM字幕组"}
	it := &domain.Item{Title: "dummy", Subgroup: "TSDM字幕组", Torrent: "magnet:?xt=urn:btih:abc"}
	if !RenameWithEpisode(ani, it, cfg, 2) {
		t.Fatal("RenameWithEpisode 应成功")
	}
	if it.Episode != 1 {
		t.Errorf("offset=-1 时给定 ep=2 应得 1, 实际 %v", it.Episode)
	}
	if it.ReName != "测试番剧 S01E01" {
		t.Errorf("ReName = %q, want %q", it.ReName, "测试番剧 S01E01")
	}
}

// 回归: 无集数(ep<=0)应返回 false（条目被丢弃）。
func TestRenameWithEpisodeRejectsNoEpisode(t *testing.T) {
	cfg := testConfig()
	ani := &domain.Ani{Season: 1, Offset: 0, Ova: false, Title: "测试番剧", Subgroup: "TSDM字幕组"}
	it := &domain.Item{Title: "dummy", Subgroup: "TSDM字幕组", Torrent: "magnet:?xt=urn:btih:abc"}
	if RenameWithEpisode(ani, it, cfg, 0) {
		t.Error("ep=0 应返回 false")
	}
}

// 回归: OVA 直接用剧名渲染（不附加集号）。
func TestRenameWithEpisodeOVA(t *testing.T) {
	cfg := testConfig()
	ani := &domain.Ani{Season: 1, Offset: 0, Ova: true, Title: "剧场版 测试", Subgroup: "TSDM字幕组"}
	it := &domain.Item{Title: "dummy", Subgroup: "TSDM字幕组", Torrent: "magnet:?xt=urn:btih:abc"}
	if !RenameWithEpisode(ani, it, cfg, 1) {
		t.Fatal("OVA 应成功")
	}
	if it.ReName != "剧场版 测试" {
		t.Errorf("ReName = %q, want %q", it.ReName, "剧场版 测试")
	}
}

// RenameDel: 去除年份标记（RenameDelYear 开启时）。
func TestRenameDel(t *testing.T) {
	cfg := &domain.Config{RenameDelYear: true}
	got := RenameDel("测试番剧 (2026)", cfg)
	if got != "测试番剧" {
		t.Errorf("RenameDel = %q, want %q", got, "测试番剧")
	}
	// 关闭时保留年份
	cfg.RenameDelYear = false
	if got := RenameDel("测试番剧 (2026)", cfg); got != "测试番剧 (2026)" {
		t.Errorf("关闭时 RenameDel = %q", got)
	}
}