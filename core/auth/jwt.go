package auth

import (
	"maps"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"
	"github.com/huwenlong92/sdkit/core/errors"
	"github.com/huwenlong92/sdkit/pkg/jwtx"
)

type JWTClaims = jwtx.SubjectClaims

func parseTokenWithSecret(tokenStr string, sec string, options ...jwt.ParserOption) (*JWTClaims, error) {
	claims, err := jwtx.ParseSubjectWithOptions(tokenStr, sec, options...)
	if err != nil {
		return nil, errors.ErrUnauthorized
	}
	return claims, nil
}

func identityFromClaims(claims *JWTClaims) *Identity {
	if claims == nil {
		return nil
	}
	expiresAt := time.Time{}
	if claims.ExpiresAt != nil {
		expiresAt = claims.ExpiresAt.Time
	}
	return &Identity{
		SubjectID:   claims.SubjectID,
		Subject:     claims.Subject,
		SubjectType: claims.SubjectType,
		TenantID:    claims.TenantID,
		Username:    claims.Username,
		RoleID:      claims.RoleID,
		Roles:       append([]string(nil), claims.Roles...),
		Permissions: append([]string(nil), claims.Permissions...),
		ExpiresAt:   expiresAt,
		Extra:       maps.Clone(claims.Extra),
	}
}

// ======================== 内部函数 ========================

func generateToken(sec, iss string, exp int, audience string, identity *Identity) (string, error) {
	audiences := []string(nil)
	if audience != "" {
		audiences = []string{audience}
	}
	return jwtx.SignSubject(sec, iss, exp, jwtx.Subject{
		ID:          identity.SubjectID,
		Subject:     identity.Subject,
		Type:        identity.SubjectType,
		TenantID:    identity.TenantID,
		Username:    identity.Username,
		RoleID:      identity.RoleID,
		Roles:       append([]string(nil), identity.Roles...),
		Permissions: append([]string(nil), identity.Permissions...),
		Extra:       maps.Clone(identity.Extra),
		Audience:    audiences,
	})
}
