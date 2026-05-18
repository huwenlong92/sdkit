package auth

const (
	ContextIdentityKey = "auth.identity"
	ContextIdentity    = ContextIdentityKey
)

type Mode string

const (
	ModeJWT Mode = "jwt"
)

type Credentials struct {
	Username string
	Password string
}

type Identity struct {
	SubjectID   int64
	SubjectType string
	Username    string
	RoleID      int64
	Permissions []string
	Extra       map[string]any
}

type LoginResult struct {
	Mode     Mode
	Token    string
	Identity *Identity
}
