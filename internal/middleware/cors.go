package middleware

import (
	"os"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
)

var (
	allowedOriginsOnce sync.Once
	allowedOrigins     []string
)

func getAllowedOrigins() []string {
	allowedOriginsOnce.Do(func() {
		raw := strings.TrimSpace(os.Getenv("CORS_ALLOWED_ORIGINS"))
		if raw == "" {
			allowedOrigins = []string{
				"http://localhost:3000",
				"http://127.0.0.1:3000",
				"http://localhost:5173",
				"http://127.0.0.1:5173",
			}
			return
		}

		parts := strings.Split(raw, ",")
		for _, p := range parts {
			origin := strings.TrimSpace(p)
			if origin != "" {
				allowedOrigins = append(allowedOrigins, origin)
			}
		}
	})

	return allowedOrigins
}

func isOriginAllowed(origin string) bool {
	for _, allowed := range getAllowedOrigins() {
		if strings.EqualFold(origin, allowed) {
			return true
		}
	}
	return false
}

/* CORSMiddleware 基于白名单的跨域策略 */
func CORSMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		if origin != "" && isOriginAllowed(origin) {
			c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
			c.Writer.Header().Set("Vary", "Origin")
			c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		}
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS, PATCH")

		requestedHeaders := c.Request.Header.Get("Access-Control-Request-Headers")
		baseAllowedHeaders := "Authorization, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Accept, X-Requested-With, X-API-Key, x-pixelpunk-key"
		if requestedHeaders != "" {
			c.Writer.Header().Set("Access-Control-Allow-Headers", baseAllowedHeaders+", "+requestedHeaders)
		} else {
			c.Writer.Header().Set("Access-Control-Allow-Headers", baseAllowedHeaders)
		}

		c.Writer.Header().Set("Access-Control-Expose-Headers", "Content-Disposition, Content-Type, X-Request-Id, X-Request-ID")
		c.Writer.Header().Set("Access-Control-Max-Age", "86400")

		if c.Request.Method == "OPTIONS" {
			if origin != "" && !isOriginAllowed(origin) {
				c.AbortWithStatus(403)
				return
			}
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}
