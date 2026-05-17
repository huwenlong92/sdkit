package realtime

import "time"

type Client struct {
	ID       string
	Identity *Identity
	Metadata map[string]any

	UserID       string
	TenantID     int64
	Events       map[string]bool
	Ch           chan Event
	CreatedAt    time.Time
	LastActiveAt time.Time
}

func (c *Client) Subscribed(event string) bool {
	if c == nil || event == "" {
		return false
	}
	return c.Events[event]
}

func (c *Client) CurrentIdentity() *Identity {
	if c == nil {
		return nil
	}
	if c.Identity != nil && c.Identity.Authenticated() {
		return c.Identity.Normalize()
	}
	return NewUserIdentity(c.UserID, c.TenantID)
}

func (c *Client) IdentityState() IdentityState {
	return c.CurrentIdentity().State()
}

func (c *Client) Authenticated() bool {
	return c.CurrentIdentity().Authenticated()
}
