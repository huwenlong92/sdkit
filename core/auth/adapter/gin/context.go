package gin

import (
	coreauth "github.com/huwenlong92/sdkit/core/auth"

	"github.com/gin-gonic/gin"
)

func SetIdentity(c *gin.Context, identity *coreauth.Identity) {
	if identity == nil {
		return
	}
	c.Set(coreauth.ContextIdentityKey, identity)
}

func GetIdentity(c *gin.Context) *coreauth.Identity {
	v, ok := c.Get(coreauth.ContextIdentityKey)
	if !ok {
		return nil
	}
	identity, _ := v.(*coreauth.Identity)
	return identity
}
