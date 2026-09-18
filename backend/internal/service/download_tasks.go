package service

import (
	"context"
	"crypto/sha256"
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

func (s *DownloadService) findAni(id string) *domain.Ani { return s.cfg.AniByID(id) }

func (s *DownloadService) DownloadAni(ctx context.Context, ani *domain.Ani) (resultErr error) {
	original := ani.Clone()
	defer func() {
		if resultErr != nil && !errors.Is(resultErr, context.Canceled) {
			s.notifyDownloadError(original, resultErr)
		}
	}()
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
	ctx, cancel := context.WithTimeout(ctx, time.Duration(refreshTimeout(cfg))*time.Minute)
	defer cancel()
	ctx = domain.WithCloudCache(ctx)
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
		if !taskBelongs(*task, cfg) {
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
			if taskBelongs(task, cfg) && task.Episode == item.Episode && task.State != "exhausted" && task.State != "abandoned" {
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
		task := domain.DownloadTask{AccountID: domain.CloudAccountKey(cfg, cfg.DownloadToolType), Provider: taskProvider(cfg.DownloadToolType), Hash: strings.ToLower(item.InfoHash), Episode: item.Episode, Torrent: item.Torrent, Path: savePath + "/" + item.ReName, State: "pending"}
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
	return s.saveTasks(id, []domain.DownloadTask{task})
}

// A reconciliation writes all changed tasks once; submission still persists its
// intent before the remote mutation and its result afterwards for crash recovery.
func (s *DownloadService) saveTasks(id string, tasks []domain.DownloadTask) error {
	if len(tasks) == 0 {
		return nil
	}
	return s.cfg.UpdateAni(id, func(a *domain.Ani) error {
		for _, task := range tasks {
			found, wasCompleted := false, false
			for i := range a.DownloadTasks {
				if sameTask(a.DownloadTasks[i], task) {
					wasCompleted = a.DownloadTasks[i].State == "completed"
					a.DownloadTasks[i] = task
					found = true
					break
				}
			}
			if !found {
				a.DownloadTasks = append(a.DownloadTasks, task)
			}
			if task.State == "completed" && !wasCompleted {
				if !containsFloat(a.Downloaded, task.Episode) {
					a.Downloaded = append(a.Downloaded, task.Episode)
				}
				if !containsStr(a.DownloadedHash, task.Hash) {
					a.DownloadedHash = append(a.DownloadedHash, task.Hash)
				}
				a.LastDownloadTime = domain.NowMillis()
			}
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
	if !taskBelongs(*task, s.cfg.Get()) {
		return fmt.Errorf("网盘账号已变更，旧任务已暂停")
	}
	previous := task.State
	task.State, task.Error, task.UpdatedAt = "pending", "", domain.NowMillis()
	task.Attempts++
	task.RetryAt = domain.NowMillis() + int64((time.Minute*time.Duration(1<<min(task.Attempts-1, 6)))/time.Millisecond)
	if err := s.saveTask(ani.ID, *task); err != nil {
		return err
	}
	var err error
	if submitter, ok := driver.(domain.OfflineTaskSubmitter); ok {
		var remoteID string
		remoteID, err = submitter.SubmitOfflineTask(ctx, cfg, task.Torrent, task.Path, previous == "failed")
		if remoteID != "" {
			task.RemoteID = remoteID
		}
	} else if tracker, ok := driver.(domain.OfflineTaskTracker); ok && previous == "failed" {
		err = tracker.RetryOfflineTask(ctx, cfg, task.Hash, task.Torrent, task.Path)
	} else {
		err = driver.AddOfflineTask(ctx, cfg, task.Torrent, task.Path)
	}
	task.UpdatedAt = domain.NowMillis()
	if err != nil {
		task.State, task.Error = "failed", err.Error()
		if ctx.Err() != nil || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			task.State, task.Error = "unknown", "提交结果待确认："+err.Error()
		} else if task.Attempts >= 1+max(0, cfg.DownloadRetry) {
			task.State = "exhausted"
		}
	} else {
		task.State = "submitted"
		task.SubmittedAt = domain.NowMillis()
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
		if taskBelongs(task, cfg) && task.State != "completed" && task.State != "abandoned" {
			pending = true
			break
		}
	}
	if !pending {
		return nil
	}
	var statuses []domain.OfflineTaskStatus
	var statusErr error
	if tracker, ok := driver.(domain.OfflineTaskTracker); ok {
		var err error
		statuses, err = tracker.OfflineTasks(ctx, cfg)
		if err != nil {
			statusErr = fmt.Errorf("查询云端下载状态: %w", err)
		}
	}
	byID, byHash, byURL := map[string]domain.OfflineTaskStatus{}, map[string]domain.OfflineTaskStatus{}, map[string]domain.OfflineTaskStatus{}
	for _, remote := range statuses {
		if remote.ID != "" {
			byID[remote.ID] = remote
		}
		if remote.Hash != "" {
			byHash[strings.ToLower(remote.Hash)] = remote
		}
		if remote.URL != "" {
			byURL[remote.URL] = remote
		}
	}
	changed := []domain.DownloadTask{}
	for i := range ani.DownloadTasks {
		task := &ani.DownloadTasks[i]
		if !taskBelongs(*task, cfg) || task.State == "completed" || task.State == "abandoned" {
			continue
		}
		before := *task
		remote, found := byID[task.RemoteID]
		if task.RemoteID == "" {
			remote, found = byHash[strings.ToLower(task.Hash)]
			if !found {
				remote, found = byURL[task.Torrent]
			}
		}
		if found {
			task.State, task.Error = remote.State, remote.Error
			if remote.ID != "" {
				task.RemoteID = remote.ID
			}
		}
		// Pending intent may survive a crash before its submission result was saved.
		// Only explicit remote failure is safe to retry after an ambiguous submission.
		if task.State == "pending" && task.Attempts > 0 && !found {
			task.State, task.Error = "unknown", "上次提交结果待确认，请检查云端任务"
		}
		if task.State == "failed" && task.Attempts >= 1+max(0, cfg.DownloadRetry) {
			task.State = "exhausted"
		}
		if task.State == "submitted" || task.State == "unknown" {
			if task.SubmittedAt == 0 {
				task.SubmittedAt = domain.NowMillis()
			}
			if cfg.DownloadTimeout > 0 && domain.NowMillis()-task.SubmittedAt >= int64(time.Duration(min(cfg.DownloadTimeout, 10080))*time.Minute/time.Millisecond) {
				task.State, task.Error = "unknown", "云端下载超时，等待确认；可刷新查询，不会重复提交"
			}
		}
		if *task != before {
			task.UpdatedAt = domain.NowMillis()
			changed = append(changed, *task)
			if task.State == "completed" && !containsFloat(ani.Downloaded, task.Episode) {
				ani.Downloaded = append(ani.Downloaded, task.Episode)
				s.logf("INFO", "download", "%s 第 %g 集云端下载完成", ani.Title, task.Episode)
			}
		}
	}
	if err := s.saveTasks(ani.ID, changed); err != nil {
		return errors.Join(statusErr, err)
	}
	for _, task := range changed {
		if task.State == "failed" || task.State == "exhausted" || task.State == "unknown" {
			s.notifyDownloadError(ani, fmt.Errorf("第 %g 集：%s", task.Episode, task.Error))
		}
	}
	if statusErr != nil {
		return statusErr
	}
	return s.finishSubscription(cfg, ani.ID)
}

func refreshTimeout(cfg *domain.Config) int {
	if cfg.RefreshTimeout <= 0 {
		return 5
	}
	return min(cfg.RefreshTimeout, 1440)
}

// RecoverTask only changes a failed record. It cannot transfer another account's
// tasks or restart an unconfirmed submission. Work is scheduled separately.
func (s *DownloadService) RecoverTask(ctx context.Context, id, hash string, episode float64, action string) error {
	if action != "retry" && action != "replace" {
		return fmt.Errorf("不支持的恢复操作")
	}
	select {
	case s.gate <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-s.gate }()
	cfg := s.cfg.Get()
	return s.cfg.UpdateAni(id, func(a *domain.Ani) error {
		if !a.Enable {
			return fmt.Errorf("请先启用订阅")
		}
		for _, t := range a.DownloadTasks {
			if taskBelongs(t, cfg) && t.Episode == episode && (t.State == "submitted" || t.State == "pending" || t.State == "unknown" || t.State == "completed") {
				return fmt.Errorf("该集正在处理或已完成，请先刷新确认")
			}
		}
		for i := range a.DownloadTasks {
			t := &a.DownloadTasks[i]
			if !taskBelongs(*t, cfg) || t.Hash != hash || t.Episode != episode {
				continue
			}
			if t.State != "failed" && t.State != "exhausted" {
				return fmt.Errorf("仅失败任务可重试或换源")
			}
			if action == "replace" {
				t.State, t.Error = "abandoned", "已跳过此资源，等待其他版本"
			} else {
				t.State, t.Error, t.Attempts, t.RetryAt = "failed", "", 0, 0
			}
			t.UpdatedAt = domain.NowMillis()
			return nil
		}
		return fmt.Errorf("当前账号下未找到该任务")
	})
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

func (s *DownloadService) notifyDownloadError(ani *domain.Ani, err error) {
	if s.notify == nil || ani == nil {
		return
	}
	cfg := s.cfg.Get()
	key := fmt.Sprintf("notify:error:%s:%s:%x", ani.ID, domain.CloudAccountKey(cfg, cfg.DownloadToolType), sha256.Sum256([]byte(err.Error())))
	if s.cache.Contains(key) {
		return
	}
	s.cache.Put(key, "1", 10*time.Minute)
	s.notifySend(ani, ani.Title+"："+err.Error(), true, domain.NotifyError)
}
