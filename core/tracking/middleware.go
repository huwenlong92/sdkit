package tracking

import "github.com/gin-gonic/gin"

func Middleware(configs ...Config) gin.HandlerFunc {
	cfg := DefaultConfig()
	if len(configs) > 0 {
		cfg = normalizeConfig(configs[0])
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
			trackID = generateTrackID(cfg.Generator)
		}

		c.Set(Key, trackID)
		c.Header(cfg.ResponseHeader, trackID)
		c.Request = c.Request.WithContext(WithTrackID(c.Request.Context(), trackID))
		c.Next()
	}
}

func Get(c *gin.Context) string {
	if c == nil {
		return ""
	}
	id, _ := c.Get(Key)
	trackID, _ := id.(string)
	return trackID
}
