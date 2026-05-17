package cors

import (
	"strings"

	"github.com/huwenlong92/sdkit/core/requestid"
	"github.com/huwenlong92/sdkit/core/tracking"

	"github.com/gin-gonic/gin"
)

type Option func(*Config)

type Config struct {
	Origins       []string
	Methods       []string
	Headers       []string
	ExposeHeaders []string
	MaxAge        string
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
		c.Header("Access-Control-Allow-Origin", strings.Join(cfg.Origins, ", "))
		c.Header("Access-Control-Allow-Methods", strings.Join(cfg.Methods, ", "))
		c.Header("Access-Control-Allow-Headers", strings.Join(cfg.Headers, ", "))
		c.Header("Access-Control-Expose-Headers", strings.Join(cfg.ExposeHeaders, ", "))
		c.Header("Access-Control-Max-Age", cfg.MaxAge)

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}
