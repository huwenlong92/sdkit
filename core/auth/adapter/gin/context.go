package gin

import (
	"github.com/huwenlong92/sdkit/core/auth"

	"github.com/gin-gonic/gin"
)

func SetIdentity(c *gin.Context, identity *auth.Identity) {
	if identity == nil {
		return
	}
	c.Set(auth.ContextIdentityKey, identity)
}

func GetIdentity(c *gin.Context) *auth.Identity {
	v, ok := c.Get(auth.ContextIdentityKey)
	if !ok {
		return nil
	}
	identity, _ := v.(*auth.Identity)
	return identity
}
