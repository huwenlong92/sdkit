package jwtx

import (
	"time"

	jwt "github.com/golang-jwt/jwt/v5"
)

type Subject struct {
	ID          int64
	Subject     string
	Type        string
	Username    string
	RoleID      int64
	Permissions []string
}

type SubjectClaims struct {
	SubjectID   int64    `json:"sub_id"`
	SubjectType string   `json:"sub_type"`
	Username    string   `json:"username,omitempty"`
	RoleID      int64    `json:"role_id,omitempty"`
	Permissions []string `json:"permissions,omitempty"`
	jwt.RegisteredClaims
}

func SignSubject(secret, issuer string, expireSeconds int, subject Subject) (string, error) {
	claims := &SubjectClaims{
		SubjectID:   subject.ID,
		SubjectType: subject.Type,
		Username:    subject.Username,
		RoleID:      subject.RoleID,
		Permissions: subject.Permissions,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    issuer,
			Subject:   subject.Subject,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(expireSeconds) * time.Second)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

func ParseSubject(tokenStr, secret string) (*SubjectClaims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &SubjectClaims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(secret), nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*SubjectClaims)
	if !ok || !token.Valid {
		return nil, jwt.ErrTokenInvalidClaims
	}
	return claims, nil
}
