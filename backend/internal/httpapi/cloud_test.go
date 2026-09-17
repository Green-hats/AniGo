package httpapi

import (
	"context"
	"testing"
	"time"

	"github.com/greenhats/anigo/internal/cloud"
	"github.com/greenhats/anigo/internal/cloud/driver_pikpak"
	"github.com/greenhats/anigo/internal/domain"
	"github.com/greenhats/anigo/internal/service"
)

type loginTestDriver struct {
	service.NoopDriver
	config *domain.Config
}

func (d *loginTestDriver) Login(ctx context.Context, test bool, cfg *domain.Config) (bool, error) {
	d.config = cfg.Clone()
	return cfg.PikpakPassword != "", nil
}
func TestLoginUsesUnsavedSelectedCloudWithoutSaving(t *testing.T) {
	s := newTestServer(t)
	driver := &loginTestDriver{}
	cloud.Register("pikpak", func() domain.CloudDriver { return driver })
	t.Cleanup(func() { cloud.Register("pikpak", driverpikpak.New) })
	res := decodeResult(t, doReq(t, s, "POST", "/api/downloadLoginTest", map[string]string{"downloadToolType": "pikpak", "pikpakEmail": "new@example.com", "pikpakPassword": "new-pass"}))
	if res.Code != 200 || driver.config == nil || driver.config.PikpakEmail != "new@example.com" {
		t.Fatalf("wrong login config: %+v", res)
	}
	if s.cfg.Get().DownloadToolType != "115" || s.cfg.Get().PikpakEmail != "" {
		t.Fatal("test saved credentials")
	}
	res = decodeResult(t, doReq(t, s, "POST", "/api/downloadLoginTest", map[string]string{"downloadToolType": "pikpak", "pikpakPassword": ""}))
	if res.Code == 200 {
		t.Fatal("blank credentials fell back to previous ones")
	}
}
func TestTicketsInvalidateOnCloudOrPikPakCredentialsChange(t *testing.T) {
	for _, raw := range []string{`{"downloadToolType":"pikpak"}`, `{"pikpakEmail":"changed@example.com"}`, `{"pikpakPassword":"changed"}`} {
		s := newTestServer(t)
		ticket := s.signPlayTicket("file", time.Now().Add(time.Hour))
		if !s.validPlayTicket(ticket, "file") {
			t.Fatal("ticket did not validate")
		}
		if err := s.cfg.SetConfigRaw([]byte(raw)); err != nil {
			t.Fatal(err)
		}
		if s.validPlayTicket(ticket, "file") {
			t.Fatal("old cloud ticket still accepted")
		}
	}
}
