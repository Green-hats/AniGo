package cloud

import (
	"github.com/greenhats/anigo/internal/domain"
	"testing"
)

func TestPikPakSelectionAndSwitch(t *testing.T) {
	r := NewRegistry()
	cfg := domain.DefaultConfig()
	if r.Get(cfg).Name() != "115" {
		t.Fatal("default changed")
	}
	cfg.DownloadToolType = "PikPak"
	driver := r.Get(cfg)
	if driver.Name() != "pikpak" {
		t.Fatal("PikPak fell back to 115")
	}
	if _, ok := driver.(domain.OfflineTaskTracker); !ok {
		t.Fatal("missing offline tracker")
	}
	if r.Get(cfg) != driver {
		t.Fatal("session driver not reused")
	}
	cfg.DownloadToolType = "115"
	if r.Get(cfg).Name() != "115" {
		t.Fatal("cannot switch back")
	}
}
