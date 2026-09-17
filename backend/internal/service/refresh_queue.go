package service

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
)

type RefreshJob struct {
	ID        string `json:"id"`
	State     string `json:"state"`
	Error     string `json:"error,omitempty"`
	UpdatedAt int64  `json:"updatedAt"`
}

// RefreshQueue 有界、按订阅合并任务，只保存 ID，执行时读取最新订阅。
type RefreshQueue struct {
	mu       sync.Mutex
	jobs     map[string]RefreshJob
	pending  []string
	capacity int
	wake     chan struct{}
	done     chan struct{}
	cancel   context.CancelFunc
	ctx      context.Context
	run      func(context.Context, string) error
}

func NewRefreshQueue(capacity int, run func(context.Context, string) error) *RefreshQueue {
	return &RefreshQueue{jobs: map[string]RefreshJob{}, capacity: capacity, wake: make(chan struct{}, 1), run: run}
}

func (q *RefreshQueue) Start(parent context.Context) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.cancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(parent)
	q.cancel, q.done, q.ctx = cancel, make(chan struct{}), ctx
	go q.loop(ctx, q.done)
}

func (q *RefreshQueue) Stop() {
	q.mu.Lock()
	cancel, done := q.cancel, q.done
	if cancel != nil {
		cancel()
	}
	q.mu.Unlock()
	if done != nil {
		<-done
	}
	q.mu.Lock()
	q.cancel = nil
	q.mu.Unlock()
}

func (q *RefreshQueue) Enqueue(ids ...string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.cancel == nil || q.ctx.Err() != nil {
		return fmt.Errorf("后台任务尚未启动")
	}
	fresh := []string{}
	seen := map[string]bool{}
	active := 0
	for _, job := range q.jobs {
		if job.State == "queued" || job.State == "running" {
			active++
		}
	}
	for _, id := range ids {
		job := q.jobs[id]
		if id == "" || seen[id] || job.State == "queued" || job.State == "running" {
			continue
		}
		seen[id] = true
		fresh = append(fresh, id)
	}
	if active+len(fresh) > q.capacity {
		return fmt.Errorf("刷新队列已满，请稍后重试")
	}
	for _, id := range fresh {
		q.jobs[id] = RefreshJob{ID: id, State: "queued", UpdatedAt: time.Now().UnixMilli()}
		q.pending = append(q.pending, id)
	}
	q.trimLocked()
	select {
	case q.wake <- struct{}{}:
	default:
	}
	return nil
}

func (q *RefreshQueue) Snapshot() []RefreshJob {
	q.mu.Lock()
	defer q.mu.Unlock()
	jobs := make([]RefreshJob, 0, len(q.jobs))
	for _, job := range q.jobs {
		jobs = append(jobs, job)
	}
	sort.Slice(jobs, func(i, j int) bool { return jobs[i].ID < jobs[j].ID })
	return jobs
}

func (q *RefreshQueue) trimLocked() {
	for len(q.jobs) > q.capacity*2 {
		oldest := ""
		for id, job := range q.jobs {
			if job.State == "queued" || job.State == "running" {
				continue
			}
			if oldest == "" || job.UpdatedAt < q.jobs[oldest].UpdatedAt {
				oldest = id
			}
		}
		if oldest == "" {
			break
		}
		delete(q.jobs, oldest)
	}
}

func (q *RefreshQueue) loop(ctx context.Context, done chan struct{}) {
	defer close(done)
	defer func() {
		q.mu.Lock()
		for id, job := range q.jobs {
			if job.State == "queued" || job.State == "running" {
				job.State, job.Error, job.UpdatedAt = "cancelled", "任务已取消", time.Now().UnixMilli()
				q.jobs[id] = job
			}
		}
		q.pending = nil
		q.mu.Unlock()
	}()
	for {
		if ctx.Err() != nil {
			return
		}
		q.mu.Lock()
		if len(q.pending) == 0 {
			q.mu.Unlock()
			select {
			case <-ctx.Done():
				return
			case <-q.wake:
				continue
			}
		}
		id := q.pending[0]
		q.pending = q.pending[1:]
		q.jobs[id] = RefreshJob{ID: id, State: "running", UpdatedAt: time.Now().UnixMilli()}
		q.mu.Unlock()
		err := q.execute(ctx, id)
		job := RefreshJob{ID: id, State: "completed", UpdatedAt: time.Now().UnixMilli()}
		if err != nil {
			job.State, job.Error = "failed", err.Error()
		}
		if ctx.Err() != nil {
			job.State, job.Error = "cancelled", "任务已取消"
		}
		q.mu.Lock()
		q.jobs[id] = job
		q.mu.Unlock()
	}
}

func (q *RefreshQueue) execute(ctx context.Context, id string) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("刷新任务异常: %v", r)
		}
	}()
	return q.run(ctx, id)
}

func (s *DownloadService) StartBackground(ctx context.Context) { s.queue.Start(ctx) }
func (s *DownloadService) StopBackground()                     { s.queue.Stop() }
func (s *DownloadService) RefreshStatus() []RefreshJob         { return s.queue.Snapshot() }
func (s *DownloadService) EnqueueRefresh(ids ...string) error  { return s.queue.Enqueue(ids...) }
func (s *DownloadService) EnqueueAll() error {
	ids := []string{}
	for _, ani := range s.cfg.AniList() {
		if ani != nil && ani.Enable {
			ids = append(ids, ani.ID)
		}
	}
	return s.queue.Enqueue(ids...)
}
