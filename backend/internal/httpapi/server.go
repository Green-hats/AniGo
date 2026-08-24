package httpapi

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/greenhats/anigo/internal/domain"
	"github.com/greenhats/anigo/internal/service"
)

// Server 是 Gin HTTP 应用服务。
type Server struct {
	engine *gin.Engine
	cfg    *service.ConfigService
}

// NewServer 构建注册了所有路由的 Gin 引擎。
func NewServer(cfg *service.ConfigService) *Server {
	gin.SetMode(gin.ReleaseMode)
	s := &Server{
		engine: gin.New(),
		cfg:    cfg,
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