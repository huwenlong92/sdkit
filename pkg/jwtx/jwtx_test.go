package jwtx

import "testing"

func TestSignAndParseSubject(t *testing.T) {
	token, err := SignSubject("secret", "issuer", 3600, Subject{
		ID:          1001,
		Type:        "admin",
		Username:    "root",
		RoleID:      1,
		Permissions: []string{"system:read"},
	})
	if err != nil {
		t.Fatalf("SignSubject error: %v", err)
	}

	claims, err := ParseSubject(token, "secret")
	if err != nil {
		t.Fatalf("ParseSubject error: %v", err)
	}
	if claims.SubjectID != 1001 || claims.SubjectType != "admin" {
		t.Fatalf("unexpected subject: id=%d type=%s", claims.SubjectID, claims.SubjectType)
	}
	if claims.Username != "root" || claims.RoleID != 1 {
		t.Fatalf("unexpected profile claims: username=%s role_id=%d", claims.Username, claims.RoleID)
	}
	if len(claims.Permissions) != 1 || claims.Permissions[0] != "system:read" {
		t.Fatalf("unexpected permissions: %#v", claims.Permissions)
	}
}
