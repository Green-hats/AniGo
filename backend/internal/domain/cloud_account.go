package domain

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
)

// CloudAccountKey identifies an account without persisting credentials. A password
// or cookie session rotation must not transfer tasks to a different account.
func CloudAccountKey(cfg *Config, provider string) string {
	var identity string
	if strings.EqualFold(provider, "pikpak") {
		identity = "pikpak:" + strings.ToLower(strings.TrimSpace(cfg.PikpakEmail))
	} else {
		identity = "115:cookie:" + cfg.Pan115Cookie
		for _, part := range strings.Split(cfg.Pan115Cookie, ";") {
			key, value, ok := strings.Cut(strings.TrimSpace(part), "=")
			if ok && strings.EqualFold(key, "UID") && value != "" {
				// UID is userID_deviceID_loginTime; only the first component is stable.
				identity = "115:uid:" + strings.SplitN(value, "_", 2)[0]
				break
			}
		}
	}
	return fmt.Sprintf("%x", sha256.Sum256([]byte(identity)))
}

type cloudCacheKey struct{}

// WithCloudCache allows a short shared snapshot during background refreshes.
// Interactive checks omit it and always fetch current remote state.
func WithCloudCache(ctx context.Context) context.Context {
	return context.WithValue(ctx, cloudCacheKey{}, true)
}
func CloudCacheEnabled(ctx context.Context) bool {
	enabled, _ := ctx.Value(cloudCacheKey{}).(bool)
	return enabled
}
