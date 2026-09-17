package httpapi

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/greenhats/anigo/internal/domain"
	"github.com/greenhats/anigo/internal/service"
	"github.com/greenhats/anigo/internal/store"
)

type mediaDriver struct {
	service.NoopDriver
	streamURL string
}

func (d *mediaDriver) Login(context.Context, bool, *domain.Config) (bool, error) { return true, nil }
func (d *mediaDriver) ListDir(context.Context, *domain.Config, string) ([]domain.CloudFile, error) {
	return []domain.CloudFile{{Name: "Review E01.mkv", PickCode: "file1"}}, nil
}
func (d *mediaDriver) FileURLByPickCode(context.Context, *domain.Config, string) (string, error) {
	return d.streamURL, nil
}

type mediaCloud struct{ driver *mediaDriver }

func (m mediaCloud) Get(*domain.Config) domain.CloudDriver { return m.driver }

func TestPlayTicketOnlyAuthorizesSelectedFileAndExpires(t *testing.T) {
	s := newTestServerWithPassword(t, bcryptHash(t, "secret"))
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Range") != "bytes=0-2" {
			t.Error("range not forwarded")
		}
		w.Header().Set("Content-Range", "bytes 0-2/9")
		w.Header().Set("Content-Type", "video/x-matroska")
		w.WriteHeader(http.StatusPartialContent)
		if r.Method != http.MethodHead {
			io.WriteString(w, "abc")
		}
	}))
	defer upstream.Close()
	s.download = service.NewDownloadService(s.cfg, s.rss, mediaCloud{&mediaDriver{streamURL: upstream.URL}}, store.NewTTLCache(), nil, nil, nil)
	ani := domain.DefaultAni()
	if err := s.cfg.SaveAniList([]*domain.Ani{ani}); err != nil {
		t.Fatal(err)
	}
	token := loginAndGetToken(t, s, "secret")
	w := doReqAuth(t, s, "POST", "/api/playTicket", map[string]string{"id": ani.ID, "pickCode": "file1"}, token)
	result := decodeResult(t, w)
	if result.Code != 200 {
		t.Fatal(result.Message)
	}
	link := result.Data.(map[string]interface{})["url"].(string)
	if strings.Contains(link, token) {
		t.Fatal("login token leaked into media link")
	}
	for _, method := range []string{"GET", "HEAD"} {
		req := httptest.NewRequest(method, link, nil)
		req.Header.Set("Range", "bytes=0-2")
		out := httptest.NewRecorder()
		s.Handler().ServeHTTP(out, req)
		if out.Code != 206 {
			t.Fatalf("signed %s request failed: %d %s", method, out.Code, out.Body.String())
		}
		if method == "GET" && out.Body.String() != "abc" {
			t.Fatal("stream body mismatch")
		}
	}
	for _, link := range []string{
		"/api/file?pickcode=file1",
		strings.Replace(link, "pickcode=file1", "pickcode=file2", 1),
		"/api/file?pickcode=file1&ticket=" + s.signPlayTicket("file1", time.Now().Add(-time.Second)),
	} {
		if got := doReq(t, s, "GET", link, nil).Code; got != 401 {
			t.Fatalf("invalid ticket accepted: %d", got)
		}
	}
	ticket := s.signPlayTicket("file1", time.Now().Add(time.Hour))
	if res := decodeResult(t, doReq(t, s, "POST", "/api/config?ticket="+ticket, nil)); res.Code != 401 {
		t.Fatal("media ticket authorized config API")
	}
	bad := doReqAuth(t, s, "POST", "/api/playTicket", map[string]string{"id": ani.ID, "pickCode": "other"}, token)
	if decodeResult(t, bad).Code == 200 {
		t.Fatal("ticket issued for unrelated file")
	}
	if err := s.cfg.SetConfigRaw([]byte(`{"pan115Cookie":"changed"}`)); err != nil {
		t.Fatal(err)
	}
	if s.validPlayTicket(ticket, "file1") {
		t.Fatal("old account ticket still valid")
	}
}

func TestStreamProxyCancelsUpstream(t *testing.T) {
	entered := make(chan struct{})
	cancelled := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(entered); <-r.Context().Done(); close(cancelled) }))
	defer upstream.Close()
	s := newTestServer(t)
	s.download = service.NewDownloadService(s.cfg, s.rss, mediaCloud{&mediaDriver{streamURL: upstream.URL}}, store.NewTTLCache(), nil, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequest("GET", "/api/file?pickcode=file1", nil).WithContext(ctx)
	done := make(chan struct{})
	go func() { s.Handler().ServeHTTP(httptest.NewRecorder(), req); close(done) }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("upstream not called")
	}
	cancel()
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("upstream not cancelled")
	}
	<-done
}
