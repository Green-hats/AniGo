package notifier

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/greenhats/anigo/internal/domain"
)

const deliveryLimit = 256

type DeliveryRecord struct {
	ID        string `json:"id"`
	Channel   string `json:"channel"`
	Title     string `json:"title"`
	State     string `json:"state"`
	Attempts  int    `json:"attempts"`
	Error     string `json:"error,omitempty"`
	UpdatedAt int64  `json:"updatedAt"`
}
type delivery struct {
	DeliveryRecord
	cfg  domain.NotificationConfig
	note domain.Notification
}

func (r *Registry) Start(parent context.Context) {
	r.lifecycle.Lock()
	defer r.lifecycle.Unlock()
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(parent)
	q := make(chan string, 128)
	r.ctx, r.cancel, r.queue = ctx, cancel, q
	for i := 0; i < 2; i++ {
		r.wg.Add(1)
		go r.worker(ctx, q)
	}
}
func (r *Registry) Stop() {
	r.lifecycle.Lock()
	defer r.lifecycle.Unlock()
	r.mu.Lock()
	if r.cancel == nil {
		r.mu.Unlock()
		return
	}
	r.cancel()
	r.mu.Unlock()
	r.wg.Wait()
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, d := range r.records {
		if d.State == "queued" || d.State == "sending" {
			d.State, d.Error, d.UpdatedAt = "cancelled", "服务已停止", domain.NowMillis()
		}
	}
	r.cancel = nil
}
func (r *Registry) trimLocked() {
	if len(r.records) < deliveryLimit {
		return
	}
	oldest := ""
	for id, d := range r.records {
		if d.State == "queued" || d.State == "sending" {
			continue
		}
		if oldest == "" || d.UpdatedAt < r.records[oldest].UpdatedAt {
			oldest = id
		}
	}
	if oldest != "" {
		delete(r.records, oldest)
	}
}
func (r *Registry) enqueueLocked(d *delivery) error {
	if r.cancel == nil || r.ctx.Err() != nil {
		return errors.New("通知服务尚未运行")
	}
	select {
	case r.queue <- d.ID:
		d.State, d.Error, d.Attempts, d.UpdatedAt = "queued", "", 0, domain.NowMillis()
		return nil
	default:
		return errors.New("通知队列已满，可稍后补发")
	}
}
func (r *Registry) Dispatch(ctx context.Context, cfg *domain.Config, n *domain.Notification) {
	if ctx.Err() != nil || (n.Ani != nil && !n.Ani.Message) {
		return
	}
	list := append([]domain.NotificationConfig(nil), cfg.NotificationConfigList...)
	sort.SliceStable(list, func(i, j int) bool { return list[i].Sort < list[j].Sort })
	for _, nc := range list {
		if !nc.Enable || r.Get(nc.NotificationType) == nil {
			continue
		}
		matched := false
		for _, s := range nc.StatusList {
			if s == n.Status {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		note := *n
		note.Ani = n.Ani.Clone()
		nc.StatusList = append([]domain.NotificationStatusEnum(nil), nc.StatusList...)
		d := &delivery{DeliveryRecord: DeliveryRecord{ID: domain.NewUUID(), Channel: string(nc.NotificationType), Title: aniTitle(n.Ani), UpdatedAt: domain.NowMillis()}, cfg: nc, note: note}
		r.mu.Lock()
		r.trimLocked()
		r.records[d.ID] = d
		err := r.enqueueLocked(d)
		if err != nil {
			d.State, d.Error = "failed", err.Error()
		}
		r.mu.Unlock()
		if err != nil && r.logFn != nil {
			r.logFn(fmt.Sprintf("通知 %s 发送失败: %s", d.Channel, err))
		}
	}
}
func (r *Registry) Records() []DeliveryRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]DeliveryRecord, 0, len(r.records))
	for _, d := range r.records {
		out = append(out, d.DeliveryRecord)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt > out[j].UpdatedAt })
	return out
}
func (r *Registry) Retry(id string) error {
	cfg := r.base.fetcher.Cfg.Get()
	r.mu.Lock()
	defer r.mu.Unlock()
	d := r.records[id]
	if d == nil {
		return errors.New("通知记录已过期或不存在")
	}
	if d.State != "failed" && d.State != "cancelled" {
		return errors.New("仅失败或取消的通知可补发")
	}
	valid := false
	for _, nc := range cfg.NotificationConfigList {
		if nc.Enable && reflect.DeepEqual(nc, d.cfg) {
			valid = true
			break
		}
	}
	if !valid {
		return errors.New("通知渠道已修改或停用，请先测试当前渠道")
	}
	return r.enqueueLocked(d)
}
func (r *Registry) worker(ctx context.Context, q <-chan string) {
	defer r.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case id := <-q:
			if ctx.Err() != nil {
				return
			}
			r.send(ctx, id)
		}
	}
}
func (r *Registry) send(ctx context.Context, id string) {
	r.mu.Lock()
	d := r.records[id]
	if d == nil {
		r.mu.Unlock()
		return
	}
	d.State = "sending"
	nc, n := d.cfg, d.note
	r.mu.Unlock()
	var err error
	for i := 0; i < max(1, min(nc.Retry, 10)); i++ {
		if i > 0 {
			timer := time.NewTimer(r.backoff * time.Duration(1<<min(i-1, 6)))
			select {
			case <-ctx.Done():
				timer.Stop()
				err = ctx.Err()
			case <-timer.C:
			}
			if ctx.Err() != nil {
				break
			}
		}
		if ctx.Err() != nil {
			err = ctx.Err()
			break
		}
		r.mu.Lock()
		d.Attempts++
		d.UpdatedAt = domain.NowMillis()
		r.mu.Unlock()
		attempt, cancel := context.WithTimeout(ctx, 20*time.Second)
		err = r.Get(nc.NotificationType).Send(attempt, &nc, &n)
		cancel()
		if err == nil {
			break
		}
	}
	state, message := "sent", ""
	if err != nil {
		state, message = "failed", redactDeliveryError(err, nc)
	}
	if ctx.Err() != nil {
		state, message = "cancelled", "服务已停止"
	}
	r.mu.Lock()
	d.State, d.Error, d.UpdatedAt = state, message, domain.NowMillis()
	r.mu.Unlock()
	if err != nil && r.logFn != nil {
		r.logFn(fmt.Sprintf("通知 %s 发送失败: %s", nc.NotificationType, message))
	}
}
func redactDeliveryError(err error, cfg domain.NotificationConfig) string {
	msg := err.Error()
	for _, secret := range []string{cfg.TelegramBotToken, cfg.ServerChanSendKey, cfg.BarkDeviceKeys, cfg.WebHookUrl, cfg.WebHookHeader, cfg.Shell} {
		if secret != "" {
			msg = strings.ReplaceAll(msg, secret, "[已隐藏]")
		}
	}
	if len(msg) > 500 {
		msg = msg[:500]
	}
	return msg
}
