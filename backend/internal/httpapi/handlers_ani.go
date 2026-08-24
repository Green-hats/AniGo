package httpapi

import (
	"io"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/greenhats/anigo/internal/domain"
)

func (s *Server) handleListAni(c *gin.Context) {
	ok(c, s.ani.ListAni())
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
	s.ani.DeleteAni(ids)
	okMsg(c, "删除订阅成功")
}

func (s *Server) handleBatchEnable(c *gin.Context) {
	var ids []string
	if !readJSONOrFail(c, &ids) {
		return
	}
	value, _ := strconv.ParseBool(c.Query("value"))
	s.ani.BatchEnable(ids, value)
	okMsg(c, "修改完成")
}

func (s *Server) handlePreviewAni(c *gin.Context) {
	var body domain.Ani
	if !readJSONOrFail(c, &body) {
		return
	}
	ok(c, s.ani.PreviewAni(&body))
}

func (s *Server) handleDownloadPath(c *gin.Context) {
	var body domain.Ani
	if !readJSONOrFail(c, &body) {
		return
	}
	ok(c, s.ani.DownloadPathPreview(&body))
}

// readJSONOrFail 解码请求体，失败时写 500 错误并返回 false。
func readJSONOrFail(c *gin.Context, v interface{}) bool {
	if err := c.ShouldBindJSON(v); err != nil {
		fail(c, err.Error())
		return false
	}
	return true
}