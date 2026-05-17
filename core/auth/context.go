package auth

type identityContext interface {
	Get(any) (any, bool)
}

func CurrentIdentity(c identityContext) *Identity {
	if c == nil {
		return nil
	}
	v, ok := c.Get(ContextIdentityKey)
	if !ok {
		return nil
	}
	identity, _ := v.(*Identity)
	return identity
}

func UserID(c identityContext) int64 {
	if identity := CurrentIdentity(c); identity != nil {
		return identity.SubjectID
	}
	return 0
}

func RoleID(c identityContext) int64 {
	if identity := CurrentIdentity(c); identity != nil {
		return identity.RoleID
	}
	return 0
}

func Claims(c identityContext) *JWTClaims {
	identity := CurrentIdentity(c)
	if identity == nil {
		return nil
	}
	return &JWTClaims{
		SubjectID:   identity.SubjectID,
		SubjectType: identity.SubjectType,
		Username:    identity.Username,
		RoleID:      identity.RoleID,
		Permissions: append([]string(nil), identity.Permissions...),
	}
}
