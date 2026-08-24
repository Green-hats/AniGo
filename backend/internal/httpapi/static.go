package httpapi

import (
	"bytes"
	"embed"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

//go:embed static
var staticFS embed.FS

// staticFileServer 提供嵌入的前端构建产物（SPA 兜底到 index.html）。
func staticFileServer() gin.HandlerFunc {
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		panic(err)
	}
	index, _ := fs.ReadFile(sub, "index.html")
	serveFile := func(c *gin.Context, name string) {
		c.Request.URL.Path = name
		http.ServeContent(c.Writer, c.Request, name, time.Time{}, bytes.NewReader(mustRead(sub, name)))
	}
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		// API 路由已在上层匹配，这里只服务静态资源
		if strings.HasPrefix(path, "/api/") {
			c.Status(http.StatusNotFound)
			return
		}
		// 根路径或纯文件请求
		p := strings.TrimPrefix(path, "/")
		if p == "" {
			p = "index.html"
		}
		if _, err := fs.Stat(sub, p); err == nil {
			serveFile(c, p)
			return
		}
		// SPA 兜底：非文件路径一律返回 index.html
		c.Header("Content-Type", "text/html; charset=utf-8")
		c.Status(http.StatusOK)
		_, _ = c.Writer.Write(index)
	}
}

// mustRead 读取嵌入文件，失败时 panic（文件必须存在）。
func mustRead(f fs.FS, name string) []byte {
	b, err := fs.ReadFile(f, name)
	if err != nil {
		panic(err)
	}
	return b
}