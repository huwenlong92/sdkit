package tests

import (
	stderrors "errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	apperrors "github.com/huwenlong92/sdkit/core/errors"
	corevalidator "github.com/huwenlong92/sdkit/core/validator"

	"github.com/gin-gonic/gin"
)

func TestBindJSONSuccess(t *testing.T) {
	corevalidator.Init()
	w, c := validatorTestContext(http.MethodPost, "/", `{"name":"tester"}`)
	c.Request.Header.Set("Content-Type", "application/json")

	var req struct {
		Name string `json:"name" binding:"required"`
	}
	if err := corevalidator.BindJSON(c, &req); err != nil {
		t.Fatalf("BindJSON() error = %v", err)
	}
	if req.Name != "tester" {
		t.Fatalf("name = %q, want tester", req.Name)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("recorder status = %d, want default %d", w.Code, http.StatusOK)
	}
}

func TestBindQueryValidationError(t *testing.T) {
	corevalidator.Init()
	_, c := validatorTestContext(http.MethodGet, "/?page=0", "")

	var req struct {
		Page int `form:"page" binding:"required,min=1"`
	}
	err := corevalidator.BindQuery(c, &req)
	if err == nil {
		t.Fatal("BindQuery() error = nil, want validation error")
	}
	var appErr *apperrors.AppError
	if !stderrors.As(err, &appErr) {
		t.Fatalf("BindQuery() error = %T, want AppError", err)
	}
	if appErr.Code != apperrors.CodeBadRequest {
		t.Fatalf("code = %d, want %d", appErr.Code, apperrors.CodeBadRequest)
	}
}

func validatorTestContext(method, target, body string) (*httptest.ResponseRecorder, *gin.Context) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, target, strings.NewReader(body))
	return w, c
}
