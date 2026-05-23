package auth

import (
	"strconv"
	"strings"
	"time"
)

const (
	ContextIdentityKey = "auth.identity"
	ContextIdentity    = ContextIdentityKey
)

const (
	MethodJWT       = "jwt"
	MethodSession   = "session"
	MethodAnonymous = "anonymous"
)

type JWTConfig struct {
	Secret string `mapstructure:"secret" yaml:"secret"`
	Issuer string `mapstructure:"issuer" yaml:"issuer"`
	Expire int    `mapstructure:"expire" yaml:"expire"`
}

type Identity struct {
	SubjectID   int64
	Subject     string
	SubjectType string
	TenantID    int64
	Username    string
	RoleID      int64
	Roles       []string
	Permissions []string
	Method      string
	Provider    string
	SessionID   string
	TokenID     string
	ExpiresAt   time.Time
	Extra       map[string]any
}

func (i *Identity) SubjectKey() string {
	if i == nil {
		return ""
	}
	if value := strings.TrimSpace(i.Subject); value != "" {
		return value
	}
	if i.SubjectID > 0 {
		return strconv.FormatInt(i.SubjectID, 10)
	}
	return ""
}

func (i *Identity) Authenticated() bool {
	return i != nil && i.SubjectKey() != ""
}

type LoginResult struct {
	Token    string
	Identity *Identity
}
