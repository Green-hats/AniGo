package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/greenhats/anigo/internal/domain"
	"github.com/greenhats/anigo/internal/rename"
)

// SyncDownload 同步执行一轮；生产入口统一通过 RefreshQueue 调度。
func (s *DownloadService) SyncDownload(ctx context.Context, list []*domain.Ani) {
	for _, ani := range list {
		if ctx.Err() != nil {
			return
		}
		if ani != nil && ani.Enable {
			if err := s.DownloadAni(ctx, ani); err != nil {
				s.logf("ERROR", "download", "%s: %v", ani.Title, err)
			}
		}
	}
}

func (s *DownloadService) findAni(id string) *domain.Ani {
	for _, a := range s.cfg.AniList() {
		if a != nil && a.ID == id {
			return a
		}
	}
	return nil
}

func (s *DownloadService) DownloadAni(ctx context.Context, ani *domain.Ani) error {
	select {
	case s.gate <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-s.gate }()
	if err := ctx.Err(); err != nil {
		return err
	}
	ani = s.findAni(ani.ID)
	if ani == nil {
		return ErrAniNotFound
	}
	if !ani.Enable {
		return fmt.Errorf("订阅已停用")
	}
	cfg := s.cfg.Get()
	driver := s.cloud.Get(cfg)
	if driver == nil {
		return fmt.Errorf("未配置网盘驱动")
	}
	if ok, err := driver.Login(ctx, true, cfg); err != nil {
		return err
	} else if !ok {
		return fmt.Errorf("下载客户端登录失败: %s", driver.GetLoginStatus().Message)
	}
	if err := s.reconcileTasks(ctx, cfg, driver, ani); err != nil {
		return err
	}
	if latest := s.findAni(ani.ID); latest == nil || !latest.Enable {
		return nil
	}
	var failures []error
	// 失败任务保留原磁力和路径，RSS 条目移出窗口后仍能重试。
	for i := range ani.DownloadTasks {
		task := &ani.DownloadTasks[i]
		if taskProvider(task.Provider) != taskProvider(cfg.DownloadToolType) {
			continue
		}
		if containsFloat(ani.NotDownload, task.Episode) || containsFloat(ani.Downloaded, task.Episode) {
			continue
		}
		if (task.State == "pending" || task.State == "failed") && task.Attempts < 1+max(0, cfg.DownloadRetry) && domain.NowMillis() >= task.RetryAt {
			if err := s.submitTask(ctx, cfg, driver, ani, task); err != nil {
				failures = append(failures, err)
			}
		}
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	items, err := s.rss.GetItems(ctx, ani)
	if err != nil {
		return err
	}
	s.RssOmit(ani, items)
	s.RssProcrastinating(ani, items)
	savePath := GetDownloadPath(cfg, ani, s.pathResolve(ctx))
	for _, item := range items {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if containsFloat(ani.Downloaded, item.Episode) || containsFloat(ani.NotDownload, item.Episode) || containsStr(ani.DownloadedHash, strings.ToLower(item.InfoHash)) {
			continue
		}
		found := false
		for _, task := range ani.DownloadTasks {
			if taskProvider(task.Provider) == taskProvider(cfg.DownloadToolType) && task.Episode == item.Episode {
				found = true
				break
			}
		}
		if found {
			continue
		}
		if ani.DownloadNew && item.Episode != items[len(items)-1].Episode {
			continue
		}
		if !item.PubDate.Time().IsZero() && cfg.DelayedDownload > 0 && time.Since(item.PubDate.Time()) < time.Duration(cfg.DelayedDownload)*time.Minute {
			continue
		}
		task := domain.DownloadTask{Provider: taskProvider(cfg.DownloadToolType), Hash: strings.ToLower(item.InfoHash), Episode: item.Episode, Torrent: item.Torrent, Path: savePath + "/" + item.ReName, State: "pending"}
		ani.DownloadTasks = append(ani.DownloadTasks, task)
		if err := s.submitTask(ctx, cfg, driver, ani, &ani.DownloadTasks[len(ani.DownloadTasks)-1]); err != nil {
			failures = append(failures, err)
		}
	}
	currentEpisode := s.rss.CurrentEpisodeNumber(ani, items)
	err = s.cfg.UpdateAni(ani.ID, func(current *domain.Ani) error {
		current.CurrentEpisodeNumber = currentEpisode
		return nil
	})
	if err == nil {
		err = s.finishSubscription(cfg, ani.ID)
	}
	return errors.Join(append(failures, err)...)
}

func (s *DownloadService) saveTask(id string, task domain.DownloadTask) error {
	return s.cfg.UpdateAni(id, func(a *domain.Ani) error {
		found := false
		for i := range a.DownloadTasks {
			if taskProvider(a.DownloadTasks[i].Provider) == taskProvider(task.Provider) && a.DownloadTasks[i].Hash == task.Hash && a.DownloadTasks[i].Episode == task.Episode {
				a.DownloadTasks[i] = task
				found = true
				break
			}
		}
		if !found {
			a.DownloadTasks = append(a.DownloadTasks, task)
		}
		if task.State == "completed" {
			if !containsFloat(a.Downloaded, task.Episode) {
				a.Downloaded = append(a.Downloaded, task.Episode)
			}
			if !containsStr(a.DownloadedHash, task.Hash) {
				a.DownloadedHash = append(a.DownloadedHash, task.Hash)
			}
			a.LastDownloadTime = domain.NowMillis()
		}
		unique := map[float64]bool{}
		for _, ep := range a.Downloaded {
			if ep > 0 && !rename.Is5(ep) {
				unique[ep] = true
			}
		}
		a.DownloadedEps = len(unique)
		return nil
	})
}

func (s *DownloadService) submitTask(ctx context.Context, cfg *domain.Config, driver domain.CloudDriver, ani *domain.Ani, task *domain.DownloadTask) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	latest := s.findAni(ani.ID)
	if latest == nil || !latest.Enable {
		return fmt.Errorf("订阅已删除或停用")
	}
	previous := task.State
	task.State, task.Error, task.UpdatedAt = "pending", "", domain.NowMillis()
	task.Attempts++
	task.RetryAt = domain.NowMillis() + int64((time.Minute*time.Duration(1<<min(task.Attempts-1, 6)))/time.Millisecond)
	if err := s.saveTask(ani.ID, *task); err != nil {
		return err
	}
	var err error
	if tracker, ok := driver.(domain.OfflineTaskTracker); ok && previous == "failed" {
		err = tracker.RetryOfflineTask(ctx, cfg, task.Hash, task.Torrent, task.Path)
	} else {
		err = driver.AddOfflineTask(ctx, cfg, task.Torrent, task.Path)
	}
	task.UpdatedAt = domain.NowMillis()
	if err != nil {
		task.State, task.Error = "failed", err.Error()
	} else {
		task.State = "submitted"
	}
	if saveErr := s.saveTask(ani.ID, *task); saveErr != nil {
		return errors.Join(err, saveErr)
	}
	if err != nil {
		s.logf("ERROR", "download", "%s 提交失败: %v", ani.Title, err)
		return err
	}
	s.logf("INFO", "download", "%s 第 %g 集已提交，等待云端完成", ani.Title, task.Episode)
	s.notifySend(ani, fmt.Sprintf("%s 第 %g 集已提交", ani.Title, task.Episode), true, domain.NotifyDownloadStart)
	return nil
}

func (s *DownloadService) reconcileTasks(ctx context.Context, cfg *domain.Config, driver domain.CloudDriver, ani *domain.Ani) error {
	pending := false
	for _, task := range ani.DownloadTasks {
		if taskProvider(task.Provider) == taskProvider(cfg.DownloadToolType) && task.State != "completed" {
			pending = true
			break
		}
	}
	if !pending {
		return nil
	}
	tracker, ok := driver.(domain.OfflineTaskTracker)
	if !ok {
		return nil
	}
	statuses, err := tracker.OfflineTasks(ctx, cfg)
	if err != nil {
		return fmt.Errorf("查询云端下载状态: %w", err)
	}
	for i := range ani.DownloadTasks {
		task := &ani.DownloadTasks[i]
		if taskProvider(task.Provider) != taskProvider(cfg.DownloadToolType) {
			continue
		}
		if task.State == "completed" {
			continue
		}
		for _, remote := range statuses {
			if (remote.Hash == "" || !strings.EqualFold(remote.Hash, task.Hash)) && (remote.URL == "" || remote.URL != task.Torrent) {
				continue
			}
			task.State, task.Error, task.UpdatedAt = remote.State, remote.Error, domain.NowMillis()
			if err := s.saveTask(ani.ID, *task); err != nil {
				return err
			}
			if task.State == "completed" {
				if !containsFloat(ani.Downloaded, task.Episode) {
					ani.Downloaded = append(ani.Downloaded, task.Episode)
				}
				s.logf("INFO", "download", "%s 第 %g 集云端下载完成", ani.Title, task.Episode)
			}
			break
		}
	}
	return s.finishSubscription(cfg, ani.ID)
}

func (s *DownloadService) finishSubscription(cfg *domain.Config, id string) error {
	if !cfg.AutoDisabled {
		return nil
	}
	latest := s.findAni(id)
	if latest == nil || !latest.Enable || latest.TotalEpisodeNumber < 1 {
		return nil
	}
	complete := func(a *domain.Ani) bool {
		if a.TotalEpisodeNumber < 1 {
			return false
		}
		for ep := 1; ep <= a.TotalEpisodeNumber; ep++ {
			if !containsFloat(a.Downloaded, float64(ep)) {
				return false
			}
		}
		return true
	}
	if !complete(latest) {
		return nil
	}
	disabled := false
	err := s.cfg.UpdateAni(id, func(a *domain.Ani) error {
		if a.Enable && complete(a) {
			a.Enable = false
			disabled = true
		}
		return nil
	})
	if err == nil && disabled {
		s.notifySend(latest, latest.Title+" 订阅已完结", true, domain.NotifyCompleted)
	}
	return err
}

// ImportConfig 等待当前下载退出临界区，避免旧任务状态覆盖刚导入的数据。
func (s *DownloadService) ImportConfig(ctx context.Context, path string) error {
	select {
	case s.gate <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-s.gate }()
	if err := s.cfg.ImportConfig(path); err != nil {
		return err
	}
	s.playMu.Lock()
	s.playCache = map[string]*playCacheEntry{}
	s.playMu.Unlock()
	return nil
}

// Older records were created by the 115-only backend.
func taskProvider(provider string) string {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "" || provider == "pan115" {
		return "115"
	}
	return provider
}
