package notifier

import (
	"context"
	"errors"
	"github.com/greenhats/anigo/internal/domain"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type deliveryCfg struct{ cfg *domain.Config }

func (c *deliveryCfg) Get() *domain.Config { return c.cfg.Clone() }

type deliverySender struct {
	calls atomic.Int32
	fail  atomic.Bool
	block bool
	times chan time.Time
}

func (s *deliverySender) Type() domain.NotificationTypeEnum { return domain.NotifySystem }
func (s *deliverySender) Send(ctx context.Context, _ *domain.NotificationConfig, _ *domain.Notification) error {
	s.calls.Add(1)
	if s.times != nil {
		s.times <- time.Now()
	}
	if s.block {
		<-ctx.Done()
		return ctx.Err()
	}
	if s.fail.Load() {
		return errors.New("failed token=topsecret")
	}
	return nil
}
func deliverySetup(t *testing.T, sender *deliverySender) (*Registry, *domain.Config) {
	t.Helper()
	cfg := domain.DefaultConfig()
	cfg.NotificationConfigList = []domain.NotificationConfig{{Enable: true, NotificationType: domain.NotifySystem, Retry: 2, TelegramBotToken: "topsecret", StatusList: []domain.NotificationStatusEnum{domain.NotifyError}}}
	r := NewRegistry(&deliveryCfg{cfg}, nil)
	r.register(sender)
	r.backoff = 10 * time.Millisecond
	r.Start(context.Background())
	t.Cleanup(r.Stop)
	return r, cfg
}
func waitDelivery(t *testing.T, r *Registry, state string) DeliveryRecord {
	t.Helper()
	deadline := time.After(2 * time.Second)
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		for _, d := range r.Records() {
			if d.State == state {
				return d
			}
		}
		select {
		case <-deadline:
			t.Fatalf("missing state %s: %+v", state, r.Records())
		case <-ticker.C:
		}
	}
}
func TestDeliveryBackoffFailureAndManualRetry(t *testing.T) {
	s := &deliverySender{times: make(chan time.Time, 10)}
	s.fail.Store(true)
	r, cfg := deliverySetup(t, s)
	r.Dispatch(context.Background(), cfg, &domain.Notification{Text: "error", Status: domain.NotifyError})
	failed := waitDelivery(t, r, "failed")
	first, second := <-s.times, <-s.times
	if second.Sub(first) < 8*time.Millisecond || failed.Attempts != 2 {
		t.Fatal("retry did not back off")
	}
	if strings.Contains(failed.Error, "topsecret") {
		t.Fatal("notification exposed token")
	}
	s.fail.Store(false)
	if err := r.Retry(failed.ID); err != nil {
		t.Fatal(err)
	}
	sent := waitDelivery(t, r, "sent")
	if sent.Attempts != 1 || s.calls.Load() != 3 {
		t.Fatal(sent)
	}
	if err := r.Retry(sent.ID); err == nil {
		t.Fatal("sent notification can be replayed")
	}
}
func TestDeliveryQueueBoundsAndShutdown(t *testing.T) {
	s := &deliverySender{block: true}
	r, cfg := deliverySetup(t, s)
	for i := 0; i < 500; i++ {
		r.Dispatch(context.Background(), cfg, &domain.Notification{Text: "error", Status: domain.NotifyError})
	}
	if len(r.Records()) > deliveryLimit || s.calls.Load() > 2 {
		t.Fatal("unbounded workers/records")
	}
	waitDelivery(t, r, "failed")
	done := make(chan struct{})
	go func() { r.Stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("shutdown stuck")
	}
	for _, d := range r.Records() {
		if d.State == "queued" || d.State == "sending" {
			t.Fatal("unfinished shutdown")
		}
	}
	r.Start(context.Background())
	r.Stop()
}
func TestDisabledAndNonmatchingChannelsDoNotSend(t *testing.T) {
	s := &deliverySender{}
	r, cfg := deliverySetup(t, s)
	r.Dispatch(context.Background(), cfg, &domain.Notification{Status: domain.NotifyCompleted})
	cfg.NotificationConfigList[0].Enable = false
	r.Dispatch(context.Background(), cfg, &domain.Notification{Status: domain.NotifyError})
	if len(r.Records()) != 0 {
		t.Fatal("unrequested channel sent")
	}
}
