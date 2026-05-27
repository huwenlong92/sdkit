package gintracking

import (
	"github.com/gin-gonic/gin"
	"github.com/huwenlong92/sdkit/core/tracking"
)

func Middleware(configs ...tracking.Config) gin.HandlerFunc {
	cfg := tracking.DefaultConfig()
	if len(configs) > 0 {
		cfg = tracking.NormalizeConfig(configs[0])
	}

	return func(c *gin.Context) {
		if !cfg.Enabled {
			c.Next()
			return
		}

		trackID := ""
		if !cfg.ForceNew {
			trackID = c.GetHeader(cfg.Header)
		}
		if trackID == "" {
			trackID = tracking.GenerateTrackID(cfg.Generator)
		}

		c.Set(tracking.Key, trackID)
		c.Header(cfg.ResponseHeader, trackID)
		c.Request = c.Request.WithContext(tracking.WithTrackID(c.Request.Context(), trackID))
		c.Next()
	}
}

func Get(c *gin.Context) string {
	if c == nil {
		return ""
	}
	id, _ := c.Get(tracking.Key)
	trackID, _ := id.(string)
	return trackID
}
