package middleware

import (
	"os"
	"pixelpunk/pkg/errors"
	"pixelpunk/pkg/utils"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

type limiterEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

func newIPRateLimiter(rps float64, burst int, rejectMsg string) gin.HandlerFunc {
	if rps <= 0 {
		rps = 20
	}
	if burst <= 0 {
		burst = int(rps * 2)
		if burst < 1 {
			burst = 1
		}
	}

	limiters := make(map[string]*limiterEntry)
	var mu sync.Mutex

	// 后台清理，避免内存持续增长
	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			now := time.Now()
			mu.Lock()
			for ip, entry := range limiters {
				if now.Sub(entry.lastSeen) > 20*time.Minute {
					delete(limiters, ip)
				}
			}
			mu.Unlock()
		}
	}()

	return func(c *gin.Context) {
		ip := utils.GetClientIP(c)
		if ip == "" {
			ip = "unknown"
		}

		now := time.Now()
		mu.Lock()
		entry, ok := limiters[ip]
		if !ok {
			entry = &limiterEntry{
				limiter:  rate.NewLimiter(rate.Limit(rps), burst),
				lastSeen: now,
			}
			limiters[ip] = entry
		} else {
			entry.lastSeen = now
		}
		allow := entry.limiter.Allow()
		mu.Unlock()

		if !allow {
			errors.HandleError(c, errors.New(errors.CodeRateLimited, rejectMsg))
			c.Abort()
			return
		}

		c.Next()
	}
}

func getEnvFloat(key string, fallback float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil || f <= 0 {
		return fallback
	}
	return f
}

func getEnvInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	i, err := strconv.Atoi(v)
	if err != nil || i <= 0 {
		return fallback
	}
	return i
}

// APIGlobalRateLimit 全局 API 限流（按 IP）
func APIGlobalRateLimit() gin.HandlerFunc {
	rps := getEnvFloat("API_RATE_LIMIT_RPS", 8)
	burst := getEnvInt("API_RATE_LIMIT_BURST", 16)
	return newIPRateLimiter(rps, burst, "请求过于频繁，请稍后再试")
}

// UploadRateLimit 上传接口限流（按 IP）
func UploadRateLimit() gin.HandlerFunc {
	rps := getEnvFloat("UPLOAD_RATE_LIMIT_RPS", 1)
	burst := getEnvInt("UPLOAD_RATE_LIMIT_BURST", 2)
	return newIPRateLimiter(rps, burst, "上传请求过于频繁，请稍后再试")
}

// DownloadRateLimit 下载/直链访问限流（按 IP）
func DownloadRateLimit() gin.HandlerFunc {
	rps := getEnvFloat("DOWNLOAD_RATE_LIMIT_RPS", 4)
	burst := getEnvInt("DOWNLOAD_RATE_LIMIT_BURST", 8)
	return newIPRateLimiter(rps, burst, "访问过于频繁，请稍后再试")
}
