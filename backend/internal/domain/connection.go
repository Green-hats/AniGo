package domain

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
)

func ProxyKey(c *Config) string {
	b, _ := json.Marshal([]any{c.Proxy, c.ProxyHost, c.ProxyPort, c.ProxyUsername, c.ProxyPassword})
	return fmt.Sprintf("%x", sha256.Sum256(b))
}
func CloudConnectionKey(c *Config) string {
	b, _ := json.Marshal([]any{c.DownloadToolType, c.Pan115Cookie, c.PikpakEmail, c.PikpakPassword, ProxyKey(c)})
	return fmt.Sprintf("%x", sha256.Sum256(b))
}
