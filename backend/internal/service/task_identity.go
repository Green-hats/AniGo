package service

import "github.com/greenhats/anigo/internal/domain"

func taskBelongs(task domain.DownloadTask, cfg *domain.Config) bool {
	return taskProvider(task.Provider) == taskProvider(cfg.DownloadToolType) && task.AccountID == domain.CloudAccountKey(cfg, task.Provider)
}
func sameTask(a, b domain.DownloadTask) bool {
	return taskProvider(a.Provider) == taskProvider(b.Provider) && a.AccountID == b.AccountID && a.Hash == b.Hash && a.Episode == b.Episode
}
func bindLegacyTasks(list []*domain.Ani, cfg *domain.Config) bool {
	changed := false
	for _, a := range list {
		if a == nil {
			continue
		}
		if a.TaskSummary != nil {
			a.TaskSummary = nil
			changed = true
		}
		for i := range a.DownloadTasks {
			task := &a.DownloadTasks[i]
			if task.AccountID == "" {
				task.AccountID = domain.CloudAccountKey(cfg, task.Provider)
				changed = true
			}
			if task.State == "submitted" && task.SubmittedAt == 0 && task.UpdatedAt > 0 {
				task.SubmittedAt = task.UpdatedAt
				changed = true
			}
		}
	}
	return changed
}

// AniByID only clones the requested subscription.
func (s *ConfigService) AniByID(id string) *domain.Ani {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, a := range s.aniLst {
		if a != nil && a.ID == id {
			return a.Clone()
		}
	}
	return nil
}

func TaskBelongs(task domain.DownloadTask, cfg *domain.Config) bool { return taskBelongs(task, cfg) }
