package sessionx

import (
	"context"
	"time"
)

type Session struct {
	ID          string
	SubjectID   int64
	SubjectType string
	Username    string
	RoleID      int64
	Permissions []string
	ExpiresAt   time.Time
	Extra       map[string]any
}

func (s *Session) GetExtra(key string) (any, bool) {
	if s == nil || s.Extra == nil {
		return nil, false
	}
	v, ok := s.Extra[key]
	return v, ok
}

func (s *Session) GetExtraString(key string) string {
	v, ok := s.GetExtra(key)
	if !ok {
		return ""
	}
	str, _ := v.(string)
	return str
}

func (s *Session) GetExtraInt64(key string) (int64, bool) {
	v, ok := s.GetExtra(key)
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case float64:
		return int64(n), true
	case int64:
		return n, true
	case int:
		return int64(n), true
	}
	return 0, false
}

type Store interface {
	Get(ctx context.Context, id string) (*Session, bool)
	Set(ctx context.Context, s *Session, ttl time.Duration) error
	Delete(ctx context.Context, id string) error
}
