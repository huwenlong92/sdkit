package jwtx

import (
	"time"

	jwt "github.com/golang-jwt/jwt/v5"
)

type Subject struct {
	ID          int64
	Subject     string
	Type        string
	TenantID    int64
	Username    string
	RoleID      int64
	Roles       []string
	Permissions []string
	Extra       map[string]any
	Audience    []string
}

type SubjectClaims struct {
	SubjectID   int64          `json:"sub_id"`
	SubjectType string         `json:"sub_type"`
	TenantID    int64          `json:"tenant_id,omitempty"`
	Username    string         `json:"username,omitempty"`
	RoleID      int64          `json:"role_id,omitempty"`
	Roles       []string       `json:"roles,omitempty"`
	Permissions []string       `json:"permissions,omitempty"`
	Extra       map[string]any `json:"extra,omitempty"`
	jwt.RegisteredClaims
}

func SignSubject(secret, issuer string, expireSeconds int, subject Subject) (string, error) {
	claims := &SubjectClaims{
		SubjectID:   subject.ID,
		SubjectType: subject.Type,
		TenantID:    subject.TenantID,
		Username:    subject.Username,
		RoleID:      subject.RoleID,
		Roles:       subject.Roles,
		Permissions: subject.Permissions,
		Extra:       subject.Extra,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    issuer,
			Subject:   subject.Subject,
			Audience:  jwt.ClaimStrings(subject.Audience),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(expireSeconds) * time.Second)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

func ParseSubject(tokenStr, secret string) (*SubjectClaims, error) {
	return ParseSubjectWithOptions(tokenStr, secret)
}

func ParseSubjectWithOptions(tokenStr, secret string, options ...jwt.ParserOption) (*SubjectClaims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &SubjectClaims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(secret), nil
	}, options...)
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*SubjectClaims)
	if !ok || !token.Valid {
		return nil, jwt.ErrTokenInvalidClaims
	}
	return claims, nil
}
