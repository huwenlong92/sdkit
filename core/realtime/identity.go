package realtime

import (
	"strings"
)

type IdentityKind string

const (
	IdentityKindUser   IdentityKind = "user"
	IdentityKindDevice IdentityKind = "device"
)

type IdentityState string

const (
	IdentityStateAnonymous     IdentityState = "anonymous"
	IdentityStateAuthenticated IdentityState = "authenticated"
)

type Identity struct {
	ID       string         `json:"id"`
	Type     string         `json:"type,omitempty"`
	DeviceID string         `json:"device_id,omitempty"`
	Platform string         `json:"platform,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`

	TenantID int64        `json:"-"`
	Kind     IdentityKind `json:"-"`
	Subject  string       `json:"-"`
}

func NewUserIdentity(userID string, tenantID int64) *Identity {
	identity := &Identity{ID: strings.TrimSpace(userID), TenantID: tenantID}
	if identity.ID != "" {
		identity.Kind = IdentityKindUser
		identity.Type = string(IdentityKindUser)
	}
	return identity
}

func (i *Identity) Normalize() *Identity {
	if i == nil {
		return nil
	}
	i.ID = strings.TrimSpace(i.ID)
	i.Type = strings.TrimSpace(i.Type)
	i.DeviceID = strings.TrimSpace(i.DeviceID)
	i.Platform = strings.TrimSpace(i.Platform)
	i.Subject = strings.TrimSpace(i.Subject)
	if i.Kind == "" && i.ID != "" {
		i.Kind = IdentityKindUser
	}
	if i.Type == "" && i.Kind != "" {
		i.Type = string(i.Kind)
	}
	if i.ID == "" && i.Subject != "" {
		i.ID = i.Subject
	}
	if i.Subject == "" {
		i.Subject = i.ID
	}
	return i
}

func (i *Identity) Authenticated() bool {
	i = i.Normalize()
	return i != nil && (i.ID != "" || i.Subject != "")
}

func (i *Identity) Anonymous() bool {
	return !i.Authenticated()
}

func (i *Identity) State() IdentityState {
	if i.Authenticated() {
		return IdentityStateAuthenticated
	}
	return IdentityStateAnonymous
}

func IdentityFromAuthResult(result *AuthResult) *Identity {
	if result == nil {
		return nil
	}
	return NewUserIdentity(result.UserID, result.TenantID)
}
