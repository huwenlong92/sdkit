package auth

import (
	"context"
	"time"

	"github.com/huwenlong92/sdkit/core/session"

	"github.com/google/uuid"
)

type SessionGuard struct {
	store session.Store
	ttl   time.Duration
}

func NewSessionGuard(store session.Store, ttl time.Duration) *SessionGuard {
	if ttl <= 0 {
		ttl = session.SessionTTL
	}
	return &SessionGuard{store: store, ttl: ttl}
}

func (g *SessionGuard) Mode() Mode {
	return ModeSession
}

func (g *SessionGuard) Login(ctx context.Context, identity *Identity) (*LoginResult, error) {
	if identity == nil {
		return nil, ErrUnauthorized
	}
	sid := uuid.NewString()
	sess := &session.Session{
		ID:          sid,
		SubjectID:   identity.SubjectID,
		SubjectType: identity.SubjectType,
		Username:    identity.Username,
		RoleID:      identity.RoleID,
		Permissions: identity.Permissions,
		Extra:       identity.Extra,
	}
	if err := g.store.Set(ctx, sess, g.ttl); err != nil {
		return nil, err
	}
	return &LoginResult{Mode: ModeSession, SessionID: sid, Identity: identity}, nil
}

func (g *SessionGuard) Logout(ctx context.Context, credential string) error {
	if credential != "" {
		return g.store.Delete(ctx, credential)
	}
	return nil
}

func (g *SessionGuard) TTL() time.Duration {
	return g.ttl
}

func (g *SessionGuard) Refresh(ctx context.Context, sessionID string) bool {
	if g == nil || g.store == nil || sessionID == "" {
		return false
	}
	sess, ok := g.store.Get(ctx, sessionID)
	if !ok || sess == nil || time.Until(sess.ExpiresAt) >= session.RenewThreshold {
		return false
	}
	_ = g.store.Set(ctx, sess, g.ttl)
	return true
}

func (g *SessionGuard) Authenticate(ctx context.Context, credential string) (*Identity, error) {
	if g == nil || g.store == nil {
		return nil, ErrUnauthorized
	}
	sess, ok := g.store.Get(ctx, credential)
	if !ok {
		return nil, ErrUnauthorized
	}
	identity := &Identity{
		SubjectID:   sess.SubjectID,
		SubjectType: sess.SubjectType,
		Username:    sess.Username,
		RoleID:      sess.RoleID,
		Permissions: sess.Permissions,
		Extra:       sess.Extra,
	}
	return identity, nil
}
