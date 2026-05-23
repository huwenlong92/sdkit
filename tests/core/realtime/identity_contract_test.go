package realtime_test

import (
	"testing"

	"github.com/huwenlong92/sdkit/core/realtime"
)

func TestIdentityState(t *testing.T) {
	if state := (&realtime.Identity{}).State(); state != realtime.IdentityStateAnonymous {
		t.Fatalf("empty identity state: want anonymous, got %s", state)
	}
	user := realtime.NewUserIdentity("1001", 2001)
	if state := user.State(); state != realtime.IdentityStateAuthenticated {
		t.Fatalf("user identity state: want authenticated, got %s", state)
	}
	if user.Kind != realtime.IdentityKindUser {
		t.Fatalf("user identity kind: want user, got %s", user.Kind)
	}
	device := (&realtime.Identity{Kind: realtime.IdentityKindDevice, Subject: " device-1 "}).Normalize()
	if !device.Authenticated() || device.Subject != "device-1" {
		t.Fatalf("device identity: %+v", device)
	}
}

func TestClientCurrentIdentityUsesStringUserID(t *testing.T) {
	client := &realtime.Client{ID: "client-1", UserID: "1001", TenantID: 2001}

	identity := client.CurrentIdentity()
	if identity.ID != "1001" || identity.TenantID != 2001 || identity.Kind != realtime.IdentityKindUser {
		t.Fatalf("identity: %+v", identity)
	}
	if !client.Authenticated() || client.IdentityState() != realtime.IdentityStateAuthenticated {
		t.Fatalf("client state: authenticated=%v state=%s", client.Authenticated(), client.IdentityState())
	}
}

func TestClientCurrentIdentityPrefersExplicitIdentity(t *testing.T) {
	client := &realtime.Client{
		ID:       "client-1",
		UserID:   "1001",
		TenantID: 2001,
		Identity: &realtime.Identity{
			Kind:    realtime.IdentityKindDevice,
			Subject: "device-1",
		},
	}

	identity := client.CurrentIdentity()
	if identity.Kind != realtime.IdentityKindDevice || identity.Subject != "device-1" || identity.ID != "device-1" {
		t.Fatalf("explicit identity: %+v", identity)
	}
}

func TestAuthResultConvertsToIdentity(t *testing.T) {
	result := (&realtime.AuthResult{UserID: "1001", TenantID: 2001}).Identity()
	if result.ID != "1001" || result.TenantID != 2001 || result.Kind != realtime.IdentityKindUser {
		t.Fatalf("auth result identity: %+v", result)
	}
	if result := ((*realtime.AuthResult)(nil)).Identity(); result.Authenticated() {
		t.Fatalf("nil auth result identity should be anonymous: %+v", result)
	}
}
