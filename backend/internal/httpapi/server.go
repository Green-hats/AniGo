package httpapi

import (
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"

	"github.com/greenhats/anigo/internal/domain"
	"github.com/greenhats/anigo/internal/service"
)

// Server 是 Gin HTTP 应用服务。
type Server struct {
	engine *gin.Engine
	cfg    *service.ConfigService
	ani    *service.AniService
}

// NewServer 构建注册了所有路由的 Gin 引擎。
func NewServer(cfg *service.ConfigService, ani *service.AniService) *Server {
	gin.SetMode(gin.ReleaseMode)
	s := &Server{
		engine: gin.New(),
		cfg:    cfg,
		ani:    ani,
	}
	s.register()
	return s
}

// Handler 返回引擎的 http.Handler。
func (s *Server) Handler() http.Handler { return s.engine }

// register 注册所有 API 路由。
func (s *Server) register() {
	r := s.engine

	r.Use(gin.Logger(), gin.Recovery())

	// 免鉴权端点
	r.Any("/api/ping", s.handlePing)
	r.GET("/api/custom.js", s.handleCustomJs)
	r.GET("/api/custom.css", s.handleCustomCss)

	// 配置
	r.POST("/api/config", s.handleConfig)
	r.POST("/api/setConfig", s.handleSetConfig)
	r.POST("/api/clearCache", s.handleClearCache)
	r.GET("/api/exportConfig", s.handleExportConfig)
	r.POST("/api/importConfig", s.handleImportConfig)

	// 订阅
	r.POST("/api/listAni", s.handleListAni)
	r.POST("/api/addAni", s.handleAddAni)
	r.POST("/api/setAni", s.handleSetAni)
	r.POST("/api/deleteAni", s.handleDeleteAni)
	r.POST("/api/batchEnable", s.handleBatchEnable)
	r.POST("/api/refreshAni", s.handleRefreshAni)
	r.POST("/api/previewAni", s.handlePreviewAni)
	r.POST("/api/downloadPath", s.handleDownloadPath)
}

func (s *Server) handlePing(c *gin.Context) {
	ok(c, nil)
}

func (s *Server) handleConfig(c *gin.Context) {
	cfg := *s.cfg.Get()
	cfg.Login.Password = ""
	ok(c, &cfg)
}

func (s *Server) handleSetConfig(c *gin.Context) {
	raw, err := c.GetRawData()
	if err != nil {
		fail(c, err.Error())
		return
	}
	if len(raw) == 0 {
		fail(c, "body is empty")
		return
	}
	if err := s.cfg.SetConfigRaw(raw); err != nil {
		fail(c, err.Error())
		return
	}
	okMsg(c, "修改成功")
}

func (s *Server) handleClearCache(c *gin.Context) {
	s.cfg.ClearCache()
	okMsg(c, "清理完成")
}

func (s *Server) handleCustomJs(c *gin.Context) {
	js := s.cfg.Get().CustomJs
	if js == "" {
		js = "// empty js"
	}
	c.Header("Content-Type", "application/javascript; charset=utf-8")
	c.Header("Cache-Control", "no-store")
	c.String(http.StatusOK, js)
}

func (s *Server) handleCustomCss(c *gin.Context) {
	css := s.cfg.Get().CustomCss
	if css == "" {
		css = "/* empty css */"
	}
	c.Header("Content-Type", "text/css; charset=utf-8")
	c.Header("Cache-Control", "no-store")
	c.String(http.StatusOK, css)
}

// handleExportConfig 下载配置备份 zip。
func (s *Server) handleExportConfig(c *gin.Context) {
	c.Header("Content-Type", "application/zip")
	c.Header("Content-Disposition", `inline; filename="anigo.backup.zip"`)
	if err := s.cfg.ExportConfig(c.Writer); err != nil {
		fail(c, err.Error())
	}
}

// handleImportConfig 上传并恢复配置备份 zip。
func (s *Server) handleImportConfig(c *gin.Context) {
	if err := c.Request.ParseMultipartForm(50 << 20); err != nil {
		fail(c, err.Error())
		return
	}
	file, _, err := c.Request.FormFile("file")
	if err != nil {
		fail(c, "未获取到文件")
		return
	}
	defer file.Close()
	tmp := filepath.Join(os.TempDir(), "anigo-import.zip")
	f, err := os.Create(tmp)
	if err != nil {
		fail(c, err.Error())
		return
	}
	if _, err := io.Copy(f, file); err != nil {
		f.Close()
		fail(c, err.Error())
		return
	}
	f.Close()
	defer os.Remove(tmp)
	if err := s.cfg.ImportConfig(tmp); err != nil {
		fail(c, err.Error())
		return
	}
	okMsg(c, "导入成功")
}

// writeResult 以 HTTP 200 写 domain.Result（Java 总是返回 200）。
func writeResult(c *gin.Context, res *domain.Result) {
	c.JSON(http.StatusOK, res)
}

func ok(c *gin.Context, data interface{}) {
	writeResult(c, domain.NewResult(data))
}

func okMsg(c *gin.Context, msg string) {
	writeResult(c, domain.NewMessage(msg))
}

func fail(c *gin.Context, msg string) {
	writeResult(c, domain.NewError(msg))
}