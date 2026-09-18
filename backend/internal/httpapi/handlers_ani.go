package httpapi

import (
	"io"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/greenhats/anigo/internal/domain"
	"github.com/greenhats/anigo/internal/service"
)

func (s *Server) handleListAni(c *gin.Context) {
	ok(c, s.ani.ListAniView(c.Query("summary") == "true"))
}

func (s *Server) handleAddAni(c *gin.Context) {
	var body domain.Ani
	if !readJSONOrFail(c, &body) {
		return
	}
	if err := s.ani.AddAni(&body); err != nil {
		fail(c, err.Error())
		return
	}
	okMsg(c, "添加订阅成功")
}

func (s *Server) handleSetAni(c *gin.Context) {
	raw, err := io.ReadAll(c.Request.Body)
	if err != nil {
		fail(c, err.Error())
		return
	}
	if err := s.ani.SetAniRaw(raw); err != nil {
		fail(c, err.Error())
		return
	}
	okMsg(c, "修改成功")
}

func (s *Server) handleDeleteAni(c *gin.Context) {
	var ids []string
	if !readJSONOrFail(c, &ids) {
		return
	}
	if err := s.ani.DeleteAni(ids); err != nil {
		fail(c, err.Error())
		return
	}
	okMsg(c, "删除订阅成功")
}

func (s *Server) handleBatchEnable(c *gin.Context) {
	var ids []string
	if !readJSONOrFail(c, &ids) {
		return
	}
	value, _ := strconv.ParseBool(c.Query("value"))
	if err := s.ani.BatchEnable(ids, value); err != nil {
		fail(c, err.Error())
		return
	}
	okMsg(c, "修改完成")
}

func (s *Server) handlePreviewAni(c *gin.Context) {
	var body domain.Ani
	if !readJSONOrFail(c, &body) {
		return
	}
	preview, err := s.ani.PreviewAni(c.Request.Context(), &body)
	if err != nil {
		fail(c, err.Error())
		return
	}
	ok(c, preview)
}

func (s *Server) handleDownloadPath(c *gin.Context) {
	var body domain.Ani
	if !readJSONOrFail(c, &body) {
		return
	}
	ok(c, s.ani.DownloadPathPreview(c.Request.Context(), &body))
}

// readJSONOrFail 解码请求体，失败时写 500 错误并返回 false。
func readJSONOrFail(c *gin.Context, v interface{}) bool {
	if err := c.ShouldBindJSON(v); err != nil {
		fail(c, err.Error())
		return false
	}
	return true
}

func (s *Server) handleTaskHistory(c *gin.Context) {
	var body struct {
		ID     string `json:"id"`
		Offset int    `json:"offset"`
		Limit  int    `json:"limit"`
	}
	if !readJSONOrFail(c, &body) {
		return
	}
	a := s.cfg.AniByID(body.ID)
	if a == nil {
		fail(c, "订阅不存在")
		return
	}
	cfg := s.cfg.Get()
	tasks := []domain.DownloadTask{}
	for i := len(a.DownloadTasks) - 1; i >= 0; i-- {
		task := a.DownloadTasks[i]
		if service.TaskBelongs(task, cfg) {
			task.Torrent, task.Path = "", ""
			tasks = append(tasks, task)
		}
	}
	start := min(max(0, body.Offset), len(tasks))
	limit := min(max(1, body.Limit), 100)
	ok(c, gin.H{"total": len(tasks), "items": tasks[start:min(start+limit, len(tasks))]})
}
func (s *Server) handleRefreshBatch(c *gin.Context) {
	var ids []string
	if !readJSONOrFail(c, &ids) {
		return
	}
	unique := []string{}
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] {
			continue
		}
		a := s.cfg.AniByID(id)
		if a == nil || !a.Enable {
			fail(c, "所选订阅不存在或已停用")
			return
		}
		seen[id] = true
		unique = append(unique, id)
	}
	if err := s.download.EnqueueBatch(unique); err != nil {
		fail(c, err.Error())
		return
	}
	okMsg(c, "已加入刷新队列")
}
