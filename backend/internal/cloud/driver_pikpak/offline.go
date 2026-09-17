package driverpikpak

import (
	"context"
	"encoding/base32"
	"encoding/hex"
	"errors"
	"net/url"
	"strings"

	"github.com/greenhats/anigo/internal/domain"
)

type remoteTask struct {
	ID      string `json:"id"`
	Phase   string `json:"phase"`
	Message string `json:"message"`
	Params  struct {
		URL string `json:"url"`
	} `json:"params"`
}

func magnetHash(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || !strings.EqualFold(u.Scheme, "magnet") {
		return ""
	}
	for _, xt := range u.Query()["xt"] {
		if !strings.HasPrefix(strings.ToLower(xt), "urn:btih:") {
			continue
		}
		hash := strings.TrimPrefix(strings.ToLower(xt), "urn:btih:")
		if len(hash) == 40 {
			if _, err := hex.DecodeString(hash); err == nil {
				return hash
			}
		}
		if len(hash) == 32 {
			if b, err := base32.StdEncoding.DecodeString(strings.ToUpper(hash)); err == nil {
				return hex.EncodeToString(b)
			}
		}
	}
	return ""
}
func taskState(phase string) string {
	switch phase {
	case "PHASE_TYPE_COMPLETE":
		return "completed"
	case "PHASE_TYPE_ERROR":
		return "failed"
	default:
		return "submitted"
	}
}
func (p *PikPak) tasks(ctx context.Context, cfg *domain.Config) ([]remoteTask, error) {
	query := url.Values{"type": {"offline"}, "limit": {"100"}}
	out := []remoteTask{}
	seen := map[string]bool{}
	for page := 0; page < 1000; page++ {
		var result struct {
			Tasks []remoteTask `json:"tasks"`
			Next  string       `json:"next_page_token"`
		}
		if err := p.request(ctx, cfg, "GET", "/drive/v1/tasks?"+query.Encode(), nil, &result); err != nil {
			return nil, err
		}
		if result.Tasks == nil {
			return nil, errors.New("PikPak 离线任务列表响应无效")
		}
		out = append(out, result.Tasks...)
		if result.Next == "" {
			return out, nil
		}
		if seen[result.Next] {
			return nil, errors.New("PikPak 任务分页游标重复")
		}
		seen[result.Next] = true
		query.Set("next_page_token", result.Next)
	}
	return nil, errors.New("PikPak 任务分页超过上限")
}
func (p *PikPak) OfflineTasks(ctx context.Context, cfg *domain.Config) ([]domain.OfflineTaskStatus, error) {
	if err := p.lock(ctx, cfg); err != nil {
		return nil, err
	}
	defer p.unlock()
	tasks, err := p.tasks(ctx, cfg)
	if err != nil {
		return nil, err
	}
	out := make([]domain.OfflineTaskStatus, 0, len(tasks))
	for _, task := range tasks {
		state := taskState(task.Phase)
		msg := ""
		if state == "failed" {
			msg = task.Message
			if msg == "" {
				msg = "PikPak 云端离线下载失败"
			}
		}
		out = append(out, domain.OfflineTaskStatus{Hash: magnetHash(task.Params.URL), URL: task.Params.URL, State: state, Error: msg})
	}
	return out, nil
}
func (p *PikPak) add(ctx context.Context, cfg *domain.Config, magnet, dest string, retry bool) error {
	if strings.TrimSpace(magnet) == "" {
		return errors.New("下载链接为空")
	}
	tasks, err := p.tasks(ctx, cfg)
	if err != nil {
		return err
	}
	hash := magnetHash(magnet)
	for _, task := range tasks {
		if task.Params.URL != magnet && (hash == "" || magnetHash(task.Params.URL) != hash) {
			continue
		}
		if taskState(task.Phase) != "failed" {
			return nil
		}
		if !retry {
			return errors.New("PikPak 已有失败任务，等待重试")
		}
		if task.ID == "" {
			return errors.New("PikPak 重试任务缺少 ID")
		}
		// Retry the existing task in place; never delete cloud files.
		query := url.Values{"id": {task.ID}, "type": {"offline"}, "create_type": {"RETRY"}}
		return p.request(ctx, cfg, "GET", "/drive/v1/task?"+query.Encode(), nil, nil)
	}
	// Keep the same per-episode folder layout as the 115 driver.
	parent, err := p.folder(ctx, cfg, dest, true)
	if err != nil {
		return err
	}
	var result struct {
		Task remoteTask `json:"task"`
	}
	err = p.request(ctx, cfg, "POST", filesPath, map[string]any{"kind": "drive#file", "parent_id": parent, "upload_type": "UPLOAD_TYPE_URL", "url": map[string]string{"url": magnet}}, &result)
	if err != nil {
		return err
	}
	if result.Task.ID == "" {
		return errors.New("PikPak 未返回离线任务 ID")
	}
	if taskState(result.Task.Phase) == "failed" {
		return errors.New("PikPak 拒绝离线任务")
	}
	return nil
}
func (p *PikPak) AddOfflineTask(ctx context.Context, cfg *domain.Config, magnet, dest string) error {
	if err := p.lock(ctx, cfg); err != nil {
		return err
	}
	defer p.unlock()
	return p.add(ctx, cfg, magnet, dest, false)
}
func (p *PikPak) RetryOfflineTask(ctx context.Context, cfg *domain.Config, hash, magnet, dest string) error {
	if err := p.lock(ctx, cfg); err != nil {
		return err
	}
	defer p.unlock()
	return p.add(ctx, cfg, magnet, dest, true)
}
