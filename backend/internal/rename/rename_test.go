package rename

import (
	"testing"

	"github.com/greenhats/anigo/internal/domain"
)

func testConfig() *domain.Config {
	return &domain.Config{Skip5: true}
}

// 回归: 剧集提取应支持 TSDM 等字幕组的常见命名([04] / [02v2] / 第3话)。
func TestRenameEpisodeParsing(t *testing.T) {
	cfg := testConfig()
	ani := &domain.Ani{Season: 1, Offset: 0, Ova: false, Title: "测试番剧", Subgroup: "TSDM字幕组"}
	cases := []struct {
		title string
		ep    float64
		ok    bool
	}{
		{"【TSDM字幕组】[测试番剧][04][MKV][WebRip][H265-10bit 1080p AAC][简日内封字幕]", 4, true},
		{"【TSDM字幕组】[测试番剧][02v2][MKV][WebRip]", 2, true},
		{"【TSDM字幕组】[测试番剧][第3话][1080p]", 3, true},
		{"【TSDM字幕组】[测试番剧][06][MP4][1080P]", 6, true},
		{"【TSDM字幕组】[测试番剧][无集数字样]", 0, false},
	}
	for _, c := range cases {
		it := &domain.Item{Title: c.title, ReName: c.title, Subgroup: "TSDM字幕组", Torrent: "magnet:?xt=urn:btih:abc"}
		got := Rename(ani, it, cfg)
		if got != c.ok {
			t.Errorf("%s: Rename=%v, 期望 %v", c.title, got, c.ok)
			continue
		}
		if got && it.Episode != c.ep {
			t.Errorf("%s: ep=%v, 期望 %v", c.title, it.Episode, c.ep)
		}
	}
}

// 回归: Offset 偏移应叠加到解析出的集数上(用于补集数从1开始但源从N开始的情况)。
func TestRenameOffsetApplied(t *testing.T) {
	cfg := testConfig()
	ani := &domain.Ani{Season: 1, Offset: -1, Ova: false, Title: "测试番剧", Subgroup: "TSDM字幕组"}
	it := &domain.Item{Title: "【TSDM字幕组】[测试番剧][02][MKV]", Subgroup: "TSDM字幕组", Torrent: "magnet:?xt=urn:btih:abc"}
	if !Rename(ani, it, cfg) {
		t.Fatal("Rename 应成功")
	}
	if it.Episode != 1 {
		t.Errorf("offset=-1 时 [02] 应为 ep=1, 实际 %v", it.Episode)
	}
}