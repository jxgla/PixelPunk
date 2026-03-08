package utils

import (
	"net"
	"strings"

	"github.com/gin-gonic/gin"
)

func GetClientIP(c *gin.Context) string {
	// Cloudflare 优先头（适配 CF Tunnel / CDN）
	if cfIP := strings.TrimSpace(c.GetHeader("CF-Connecting-IP")); cfIP != "" {
		if ip := parseIPValue(cfIP); ip != "" {
			return ip
		}
	}

	// 尝试从 X-Forwarded-For 头获取
	if xForwardedFor := c.GetHeader("X-Forwarded-For"); xForwardedFor != "" {
		// X-Forwarded-For 可能包含多个IP，取第一个有效IP
		parts := strings.Split(xForwardedFor, ",")
		for _, p := range parts {
			if ip := parseIPValue(strings.TrimSpace(p)); ip != "" {
				return ip
			}
		}
	}

	// 尝试从 X-Real-IP 头获取
	if xRealIP := c.GetHeader("X-Real-IP"); xRealIP != "" {
		if ip := parseIPValue(strings.TrimSpace(xRealIP)); ip != "" {
			return ip
		}
	}

	// 从 RemoteAddr 获取
	if ip, _, err := net.SplitHostPort(c.Request.RemoteAddr); err == nil {
		return ip
	}

	return c.Request.RemoteAddr
}

func parseIPValue(raw string) string {
	if raw == "" {
		return ""
	}

	if ip := net.ParseIP(raw); ip != nil {
		return ip.String()
	}

	if host, _, err := net.SplitHostPort(raw); err == nil {
		if ip := net.ParseIP(host); ip != nil {
			return ip.String()
		}
	}

	return ""
}
