package service

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/greenhats/anigo/internal/domain"
	"github.com/greenhats/anigo/internal/store"
)

func TestMetadataRefreshReplacesIncorrectProgressWithZeroOrUnknown(t *testing.T) {
	for _, failed := range []bool{false, true} {
		t.Run(map[bool]string{false: "not aired", true: "unavailable"}[failed], func(t *testing.T) {
			cfg := reviewConfig(t)
			ani := domain.DefaultAni()
			ani.BgmUrl = "https://bgm.tv/subject/123"
			old := 11
			ani.BgmAiredEps = &old
			ani.DownloadedEps = 4
			if err := cfg.SaveAniList([]*domain.Ani{ani}); err != nil {
				t.Fatal(err)
			}
			meta := NewMetadataService(cfg, store.NewTTLCache())
			meta.BGM().Req = func(_ context.Context, _, rawURL string, _ io.Reader, _ string) ([]byte, int, error) {
				if strings.Contains(rawURL, "/v0/subjects/") {
					return []byte(`{"id":123,"eps":11,"total_episodes":13}`), 200, nil
				}
				if failed {
					return []byte(`unavailable`), 503, nil
				}
				return []byte(`{"total":1,"data":[{"id":1,"type":0,"airdate":"2999-01-01"}]}`), 200, nil
			}
			meta.RefreshAll(context.Background(), cfg.AniList())
			got := cfg.AniList()[0]
			if failed && got.BgmAiredEps != nil || !failed && (got.BgmAiredEps == nil || *got.BgmAiredEps != 0) {
				t.Fatalf("incorrect progress after refresh: %+v", got.BgmAiredEps)
			}
			if got.DownloadedEps != 4 {
				t.Fatal("metadata refresh changed download count")
			}
		})
	}
}

func TestAiredProgressSnapshotIsIndependent(t *testing.T) {
	cfg := reviewConfig(t)
	ani := domain.DefaultAni()
	aired := 3
	ani.BgmAiredEps = &aired
	if err := cfg.SaveAniList([]*domain.Ani{ani}); err != nil {
		t.Fatal(err)
	}
	*cfg.AniList()[0].BgmAiredEps = 12
	if *cfg.AniList()[0].BgmAiredEps != 3 {
		t.Fatal("aired progress pointer leaked from a subscription snapshot")
	}
}
