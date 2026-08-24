package httpapi

import (
	"github.com/gin-gonic/gin"

	"github.com/greenhats/anigo/internal/domain"
	"github.com/greenhats/anigo/internal/provider/bgm"
)

// handleSearchBgm 按名称搜索 BGM 番剧。
func (s *Server) handleSearchBgm(c *gin.Context) {
	var body struct {
		Text string `json:"text"`
	}
	if !readJSONOrFail(c, &body) {
		return
	}
	list, err := s.meta.BGM().Search(c.Request.Context(), body.Text)
	if err != nil {
		fail(c, err.Error())
		return
	}
	ok(c, list)
}

// handleRssToAni 从 RSS URL 构建订阅。
func (s *Server) handleRssToAni(c *gin.Context) {
	var body domain.RssToAniDTO
	if !readJSONOrFail(c, &body) {
		return
	}
	// 前端批量添加时不传 enable，默认 true
	if !body.Enable {
		body.Enable = true
	}
	ani, err := s.meta.RssToAni(c.Request.Context(), &body)
	if err != nil {
		fail(c, "RSS解析失败 "+err.Error())
		return
	}
	ok(c, ani)
}

// handleGardenList 获取 animes.garden 番剧周列表。
func (s *Server) handleGardenList(c *gin.Context) {
	list, err := s.meta.Garden().ListSubjects(c.Request.Context())
	if err != nil {
		fail(c, err.Error())
		return
	}
	// 标记已订阅的番剧
	ids := map[string]bool{}
	for _, ani := range s.cfg.AniList() {
		if ani != nil && ani.BgmUrl != "" {
			if id := bgm.GetSubjectIdByURL(ani.BgmUrl); id != "" {
				ids[id] = true
			}
		}
	}
	for _, week := range list {
		for i := range week.Subjects {
			if ids[string(week.Subjects[i].ID)] {
				week.Subjects[i].Exists = true
			}
		}
	}
	ok(c, list)
}

// handleGardenGroup 获取某番剧的字幕组资源。
func (s *Server) handleGardenGroup(c *gin.Context) {
	bgmID := c.Query("subject")
	if bgmID == "" {
		fail(c, "subject 不能为空")
		return
	}
	groups, err := s.meta.Garden().Group(c.Request.Context(), bgmID)
	if err != nil {
		fail(c, err.Error())
		return
	}
	ok(c, groups)
}

// handleGetBgmTitle 返回 BGM 显示标题。
func (s *Server) handleGetBgmTitle(c *gin.Context) {
	var body struct {
		SubjectId string `json:"subjectId"`
	}
	if !readJSONOrFail(c, &body) {
		return
	}
	info, err := s.meta.BGM().GetInfo(c.Request.Context(), body.SubjectId)
	if err != nil {
		fail(c, err.Error())
		return
	}
	ok(c, map[string]string{"title": info.NameCn, "jpTitle": info.Name})
}