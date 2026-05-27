package cors

import (
	"strings"

	"github.com/huwenlong92/sdkit/core/requestid"
	"github.com/huwenlong92/sdkit/core/tracking"

	"github.com/gin-gonic/gin"
)

type Option func(*Config)

type Config struct {
	Origins          []string
	Methods          []string
	Headers          []string
	ExposeHeaders    []string
	MaxAge           string
	AllowCredentials bool
}

func WithOrigins(origins ...string) Option {
	return func(c *Config) { c.Origins = origins }
}

func WithMethods(methods ...string) Option {
	return func(c *Config) { c.Methods = methods }
}

func WithHeaders(headers ...string) Option {
	return func(c *Config) { c.Headers = headers }
}

func WithExposeHeaders(headers ...string) Option {
	return func(c *Config) { c.ExposeHeaders = headers }
}

func WithMaxAge(seconds string) Option {
	return func(c *Config) { c.MaxAge = seconds }
}

func WithCredentials(allow bool) Option {
	return func(c *Config) { c.AllowCredentials = allow }
}

func Middleware(opts ...Option) gin.HandlerFunc {
	cfg := Config{
		Origins:       []string{"*"},
		Methods:       []string{"GET", "POST", "OPTIONS"},
		Headers:       []string{"Content-Type", "Authorization", requestid.Header, tracking.Header},
		ExposeHeaders: []string{"X-Session-Expires-At", requestid.Header, tracking.Header},
		MaxAge:        "86400",
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}

	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", allowOrigin(c.GetHeader("Origin"), cfg))
		c.Header("Access-Control-Allow-Methods", strings.Join(cfg.Methods, ", "))
		c.Header("Access-Control-Allow-Headers", strings.Join(cfg.Headers, ", "))
		c.Header("Access-Control-Expose-Headers", strings.Join(cfg.ExposeHeaders, ", "))
		c.Header("Access-Control-Max-Age", cfg.MaxAge)
		if cfg.AllowCredentials {
			c.Header("Access-Control-Allow-Credentials", "true")
			c.Header("Vary", "Origin")
		}

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}

func allowOrigin(origin string, cfg Config) string {
	if cfg.AllowCredentials && origin != "" && allowsAnyOrigin(cfg.Origins) {
		return origin
	}
	return strings.Join(cfg.Origins, ", ")
}

func allowsAnyOrigin(origins []string) bool {
	for _, origin := range origins {
		if origin == "*" {
			return true
		}
	}
	return false
}
