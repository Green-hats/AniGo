package driver115

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/greenhats/anigo/internal/domain"
)

// OfflineTasks 的状态值遵循 115 SDK：-1 失败、0/1 进行中、2 完成。
// https://github.com/OpenListTeam/115-sdk-go/blob/main/offline.go
func (p *Pan115) OfflineTasks(ctx context.Context, cfg *domain.Config) ([]domain.OfflineTaskStatus, error) {
	out := []domain.OfflineTaskStatus{}
	for page := 1; page <= 1000; page++ {
		m, err := p.reqFn(ctx, cfg, "POST", apiLixianList, url.Values{"page": {strconv.Itoa(page)}})
		if err != nil {
			return nil, err
		}
		if state, ok := m["state"].(bool); !ok || !state {
			return nil, fmt.Errorf("115 任务列表响应无效")
		}
		entries, ok := m["tasks"].([]interface{})
		if !ok && m["tasks"] != nil {
			return nil, fmt.Errorf("115 任务列表格式无效")
		}
		for _, entry := range entries {
			item, ok := entry.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("115 任务格式无效")
			}
			state := "submitted"
			switch intVal(item["status"]) {
			case 2:
				state = "completed"
			case -1:
				state = "failed"
			}
			msg := ""
			if state == "failed" {
				msg = "115 云端离线下载失败"
			}
			out = append(out, domain.OfflineTaskStatus{Hash: strVal(item["info_hash"]), URL: strVal(item["url"]), State: state, Error: msg})
		}
		if page >= intVal(m["page_count"]) {
			return out, nil
		}
	}
	return nil, fmt.Errorf("115 任务列表分页超出上限")
}

// RetryOfflineTask 仅清理已确认失败的任务记录；flag=0 保留云端文件。
// https://p115client.readthedocs.io/en/latest/reference/module/client.html
func (p *Pan115) RetryOfflineTask(ctx context.Context, cfg *domain.Config, hash, magnet, path string) error {
	tasks, err := p.OfflineTasks(ctx, cfg)
	if err != nil {
		return err
	}
	for _, task := range tasks {
		if !strings.EqualFold(task.Hash, hash) && task.URL != magnet {
			continue
		}
		if task.State != "failed" {
			return nil
		}
		if task.Hash == "" {
			return fmt.Errorf("失败任务缺少 hash，无法安全重试")
		}
		_, err := p.reqFn(ctx, cfg, "POST", "https://115.com/web/lixian/?ct=lixian&ac=task_del", url.Values{"hash[0]": {task.Hash}, "flag": {"0"}})
		if err != nil {
			return err
		}
		break
	}
	return p.AddOfflineTask(ctx, cfg, magnet, path)
}
