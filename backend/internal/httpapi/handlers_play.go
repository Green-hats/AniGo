package httpapi

import (
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// videoMimeType 按文件扩展名返回常见的视频 MIME 类型（取不到返回空串）。
func videoMimeType(path string) string {
	name := strings.ToLower(path)
	for _, ext := range []string{".mkv", ".mp4", ".ts", ".webm", ".m4v", ".avi", ".mov", ".wmv", ".flv", ".m2ts", ".rmvb"} {
		if strings.HasSuffix(name, ext) {
			switch ext {
			case ".mkv":
				return "video/x-matroska"
			case ".mp4", ".m4v":
				return "video/mp4"
			case ".ts", ".m2ts":
				return "video/mp2t"
			case ".webm":
				return "video/webm"
			case ".avi":
				return "video/x-msvideo"
			case ".mov":
				return "video/quicktime"
			case ".wmv":
				return "video/x-ms-wmv"
			case ".flv":
				return "video/x-flv"
			case ".rmvb":
				return "application/vnd.rn-realmedia-vbr"
			}
		}
	}
	return ""
}

// ua115 与 driver_115 保持一致：115 CDN 播放链接绑定该 User-Agent，
// 代理转发时必须使用相同 UA 才能取流。
const ua115 = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36"

// handleFileProxy 通过 pickcode 代理转发 115 云端文件的播放流。
// 外部播放器（mpv 等）只访问本地端点，由本服务用 115 UA 拉取 CDN 流并转发
// （支持 Range，便于 seek），从而绕过 115 CDN 的 UA 绑定与鉴权问题。
// 请求：GET /api/file?pickcode=xxx
func (s *Server) handleFileProxy(c *gin.Context) {
	pickcode := c.Query("pickcode")
	if pickcode == "" {
		fail(c, "pickcode 不能为空")
		return
	}
	rawURL, err := s.download.PlayStreamURL(c.Request.Context(), pickcode)
	if err != nil || rawURL == "" {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"message": "云端文件不可用"})
		return
	}
	method := http.MethodGet
	if c.Request.Method == http.MethodHead {
		method = http.MethodHead
	}
	req, err := http.NewRequest(method, rawURL, nil)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadGateway, gin.H{"message": err.Error()})
		return
	}
	req.Header.Set("User-Agent", ua115)
	if rng := c.GetHeader("Range"); rng != "" {
		req.Header.Set("Range", rng)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadGateway, gin.H{"message": err.Error()})
		return
	}
	defer resp.Body.Close()

	// 视频文件用可识别的 Content-Type，便于播放器识别
	if mt := videoMimeType(pickcode); mt != "" {
		c.Header("Content-Type", mt)
	} else if ct := resp.Header.Get("Content-Type"); ct != "" {
		c.Header("Content-Type", ct)
	}
	if cr := resp.Header.Get("Content-Range"); cr != "" {
		c.Header("Content-Range", cr)
	}
	if l := resp.Header.Get("Content-Length"); l != "" && !strings.EqualFold(l, "0") {
		c.Header("Content-Length", l)
	}
	if ar := resp.Header.Get("Accept-Ranges"); ar != "" {
		c.Header("Accept-Ranges", ar)
	}
	c.Header("Cache-Control", "no-store")
	c.Status(resp.StatusCode)
	if c.Request.Method != http.MethodHead {
		_, _ = io.Copy(c.Writer, resp.Body)
	}
}