package archery

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

// Config 是连接 Archery 所需的全部配置。
type Config struct {
	BaseURL            string
	Username           string
	Password           string
	AuthMode           string
	ReadOnly           bool
	InsecureSkipVerify bool
	Timeout            time.Duration
	QueryTimeout       time.Duration
	TwoFAPhone         string
	TwoFAType          string
	DefaultLimit       int
	// KeepAlive 是会话心跳间隔：零值取默认 120 秒，负数表示关闭自动续期。
	KeepAlive time.Duration
	// SessionStore 为 nil 时不复用历史会话（连接测试走这条路径）。
	SessionStore *SessionStore
}

// DefaultKeepAlive 是会话心跳的默认间隔。
// 必须显著小于 Archery 侧 SESSION_COOKIE_AGE，留出多次重试余量；
// 该请求是登录态 GET，对服务端几乎无成本。
const DefaultKeepAlive = 120 * time.Second

// maxKeepAlive 限制配置上界，避免写成天文数字后实际上不再续期。
const maxKeepAlive = 24 * time.Hour

// Normalize 填充默认值，保证零值配置也能安全使用。
func (c Config) Normalize() Config {
	c.BaseURL = strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	c.Username = strings.TrimSpace(c.Username)
	c.AuthMode = strings.ToLower(strings.TrimSpace(c.AuthMode))
	if c.AuthMode == "" {
		c.AuthMode = "session"
	}
	c.TwoFAType = strings.ToLower(strings.TrimSpace(c.TwoFAType))
	if c.TwoFAType != "sms" {
		c.TwoFAType = "totp"
	}
	if c.Timeout <= 0 {
		c.Timeout = 20 * time.Second
	}
	if c.QueryTimeout <= 0 {
		c.QueryTimeout = 120 * time.Second
	}
	if c.DefaultLimit <= 0 {
		c.DefaultLimit = 100
	}
	if c.DefaultLimit > 10000 {
		c.DefaultLimit = 10000
	}
	switch {
	case c.KeepAlive < 0:
		// 显式关闭：保留负值，调用方据此跳过心跳。
		c.KeepAlive = -1
	case c.KeepAlive == 0:
		c.KeepAlive = DefaultKeepAlive
	case c.KeepAlive > maxKeepAlive:
		c.KeepAlive = maxKeepAlive
	}
	return c
}

// SessionMode 表示使用 Django 会话（Cookie + CSRF）。
func (c Config) SessionMode() bool { return c.AuthMode != "jwt" }

// BuildBaseURL 拼接协议、主机、端口和上下文路径。
func BuildBaseURL(scheme, host string, port int, basePath string) (string, error) {
	scheme = strings.ToLower(strings.TrimSpace(scheme))
	if scheme == "" {
		scheme = "https"
	}
	if scheme != "http" && scheme != "https" {
		return "", apiError(KindInvalidConfig, fmt.Sprintf("unsupported scheme %q", scheme))
	}
	host = strings.TrimSpace(host)
	host = strings.TrimPrefix(host, "http://")
	host = strings.TrimPrefix(host, "https://")
	host = strings.TrimSuffix(host, "/")
	if host == "" {
		return "", apiError(KindInvalidConfig, "Archery host is missing")
	}
	if port < 0 || port > 65535 {
		return "", apiError(KindInvalidConfig, fmt.Sprintf("invalid port %d", port))
	}
	base := basePath
	if base != "" {
		if !strings.HasPrefix(base, "/") {
			base = "/" + base
		}
		base = strings.TrimRight(base, "/")
	}
	authority := host
	if port > 0 && !strings.Contains(host, ":") {
		authority = fmt.Sprintf("%s:%d", host, port)
	}
	raw := fmt.Sprintf("%s://%s%s", scheme, authority, base)
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return "", apiError(KindInvalidConfig, "invalid Archery base URL")
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}
