package httpapi

import (
	"io"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/greenhats/anigo/internal/domain"
	"github.com/greenhats/anigo/internal/service"
)

// Server 是 Gin HTTP 应用服务。
type Server struct {
	engine       *gin.Engine
	cfg          *service.ConfigService
	ani          *service.AniService
	rss          *service.RssService
	download     *service.DownloadService
	meta         *service.MetadataService
	notify       *service.NotifyService
	logs         *service.LogService
	status       *service.StatusService
	sessions     *sessionStore
	playSecret   []byte
	streamClient *http.Client
}

// NewServer 构建注册了所有路由的 Gin 引擎。
func NewServer(cfg *service.ConfigService, ani *service.AniService, rss *service.RssService, download *service.DownloadService, meta *service.MetadataService, notify *service.NotifyService, logs *service.LogService, status *service.StatusService) *Server {
	gin.SetMode(gin.ReleaseMode)
	secret, err := newToken()
	if err != nil {
		panic(err)
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = 15 * time.Second
	s := &Server{
		engine:       gin.New(),
		cfg:          cfg,
		ani:          ani,
		rss:          rss,
		download:     download,
		meta:         meta,
		notify:       notify,
		logs:         logs,
		status:       status,
		sessions:     newSessionStore(),
		playSecret:   []byte(secret),
		streamClient: &http.Client{Transport: transport},
	}
	s.register()
	return s
}

// Handler 返回引擎的 http.Handler。
func (s *Server) Handler() http.Handler { return s.engine }

// register 注册所有 API 路由。
func (s *Server) register() {
	r := s.engine

	r.Use(gin.LoggerWithConfig(gin.LoggerConfig{SkipPaths: []string{"/api/file"}}), gin.Recovery())

	// 鉴权（白名单端点放行，其余 /api 需登录）
	r.Use(s.authMiddleware())

	// 免鉴权端点
	r.Any("/api/ping", s.handlePing)
	r.POST("/api/login", s.handleLogin)
	r.GET("/api/custom.js", s.handleCustomJs)
	r.GET("/api/custom.css", s.handleCustomCss)

	// 登录态
	r.POST("/api/logout", s.handleLogout)
	r.POST("/api/checkLogin", s.handleCheckLogin)

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
	r.POST("/api/recoverTask", s.handleRecoverTask)
	r.POST("/api/previewAni", s.handlePreviewAni)
	r.POST("/api/downloadPath", s.handleDownloadPath)

	// 下载
	r.POST("/api/downloadLoginTest", s.handleDownloadLoginTest)
	r.POST("/api/downloadStatus", s.handleDownloadStatus)
	r.POST("/api/refreshAll", s.handleRefreshAll)
	r.POST("/api/refreshStatus", func(c *gin.Context) { ok(c, s.download.RefreshStatus()) })
	r.POST("/api/deleteTorrent", s.handleDeleteTorrent)
	r.POST("/api/playList", s.handlePlayList)
	r.GET("/api/file", s.handleFileProxy)
	r.HEAD("/api/file", s.handleFileProxy)
	r.POST("/api/playTicket", s.handlePlayTicket)

	// AI
	r.POST("/api/aiPing", s.handleAIPing)

	// 元数据
	r.POST("/api/searchBgm", s.handleSearchBgm)
	r.POST("/api/rssToAni", s.handleRssToAni)
	r.POST("/api/gardenList", s.handleGardenList)
	r.POST("/api/gardenGroup", s.handleGardenGroup)
	r.POST("/api/getBgmTitle", s.handleGetBgmTitle)

	// 通知
	r.POST("/api/testNotification", s.handleTestNotification)

	// 日志
	r.POST("/api/logs", s.handleLogs)
	r.POST("/api/clearLogs", s.handleClearLogs)

	// 状态
	r.POST("/api/status", s.handleStatus)

	// 前端静态资源（SPA 兜底）
	r.NoRoute(staticFileServer())
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
	s.meta.Reload()
	s.logs.Reload(s.cfg.Dir(), s.cfg.Get())
	okMsg(c, "修改成功")
}

func (s *Server) handleClearCache(c *gin.Context) {
	s.cfg.ClearCache()
	s.rss.ReloadAI()
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

// handleAIPing 测试 AI 连通性与密钥。
func (s *Server) handleAIPing(c *gin.Context) {
	reply, err := s.rss.AIPing(c.Request.Context())
	if err != nil {
		fail(c, err.Error())
		return
	}
	ok(c, map[string]string{"reply": reply})
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
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 50<<20)
	if err := c.Request.ParseMultipartForm(8 << 20); err != nil {
		fail(c, err.Error())
		return
	}
	defer c.Request.MultipartForm.RemoveAll()
	file, _, err := c.Request.FormFile("file")
	if err != nil {
		fail(c, "未获取到文件")
		return
	}
	defer file.Close()
	f, err := os.CreateTemp("", "anigo-import-*.zip")
	if err != nil {
		fail(c, err.Error())
		return
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, err := io.Copy(f, file); err != nil {
		f.Close()
		fail(c, err.Error())
		return
	}
	if err := f.Close(); err != nil {
		fail(c, err.Error())
		return
	}
	if err := s.download.ImportConfig(c.Request.Context(), tmp); err != nil {
		fail(c, err.Error())
		return
	}
	s.logs.Reload(s.cfg.Dir(), s.cfg.Get())
	s.rss.ReloadAI()
	s.meta.Reload()
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
