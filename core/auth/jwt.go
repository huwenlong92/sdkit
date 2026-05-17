package auth

import (
	"github.com/huwenlong92/sdkit/core/errors"
	"github.com/huwenlong92/sdkit/pkg/jwtx"
)

type JWTClaims = jwtx.SubjectClaims

func parseTokenWithSecret(tokenStr string, sec string) (*JWTClaims, error) {
	claims, err := jwtx.ParseSubject(tokenStr, sec)
	if err != nil {
		return nil, errors.ErrUnauthorized
	}
	return claims, nil
}

func identityFromClaims(claims *JWTClaims) *Identity {
	if claims == nil {
		return nil
	}
	return &Identity{
		SubjectID:   claims.SubjectID,
		SubjectType: claims.SubjectType,
		Username:    claims.Username,
		RoleID:      claims.RoleID,
		Permissions: claims.Permissions,
	}
}

// ======================== 内部函数 ========================

func generateToken(sec, iss string, exp int, identity *Identity) (string, error) {
	return jwtx.SignSubject(sec, iss, exp, jwtx.Subject{
		ID:          identity.SubjectID,
		Type:        identity.SubjectType,
		Username:    identity.Username,
		RoleID:      identity.RoleID,
		Permissions: identity.Permissions,
	})
}
