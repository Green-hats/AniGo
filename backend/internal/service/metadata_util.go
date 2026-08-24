package service

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/greenhats/anigo/internal/util"
)

const minute = time.Minute

// FetchImage 下载图片字节（用普通 HTTP 客户端）。
func (s *MetadataService) FetchImage(rawURL string) ([]byte, error) {
	return util.GetBytes(s.cfg.Get(), rawURL)
}

// coverRelPath 根据图片 URL 生成 files/ 下的相对路径。
func coverRelPath(imageURL string) string {
	hash := util.MD5Hex(imageURL)
	ext := coverExt(imageURL)
	if ext == "" {
		ext = ".jpg"
	}
	return filepath.ToSlash(filepath.Join(hash[:1], hash+ext))
}

// writeFileAtomic 原子写文件。
func writeFileAtomic(fullPath string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		return err
	}
	tmp := fullPath + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, fullPath)
}

func strconvInt(i int) string {
	return strconv.Itoa(i)
}

// coverExt 从图片 URL 推断扩展名。
func coverExt(rawURL string) string {
	p := rawURL
	if i := strings.Index(p, "?"); i >= 0 {
		p = p[:i]
	}
	switch strings.ToLower(filepath.Ext(p)) {
	case ".png", ".jpg", ".jpeg", ".webp", ".gif":
		if strings.HasSuffix(strings.ToLower(p), ".jpeg") {
			return ".jpg"
		}
		return strings.ToLower(filepath.Ext(p))
	}
	return ""
}