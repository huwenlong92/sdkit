package tests

import (
	"net/http/httptest"
	"testing"

	"github.com/huwenlong92/sdkit/core/auth"
	authgin "github.com/huwenlong92/sdkit/core/auth/adapter/gin"

	"github.com/gin-gonic/gin"
)

func TestContextHelpersReadGinIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	authgin.SetIdentity(c, &auth.Identity{
		SubjectID:   42,
		SubjectType: "user",
		Username:    "tester",
		RoleID:      7,
		Permissions: []string{"read"},
	})

	if got := auth.UserID(c); got != 42 {
		t.Fatalf("UserID() = %d, want 42", got)
	}
	claims := auth.Claims(c)
	if claims == nil || claims.SubjectID != 42 || claims.RoleID != 7 {
		t.Fatalf("Claims() = %+v", claims)
	}
}
