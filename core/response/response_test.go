package response

import (
	"encoding/json"
	stderrors "errors"
	"net/http"
	"net/http/httptest"
	"testing"

	apperrors "github.com/huwenlong92/sdkit/core/errors"

	"github.com/gin-gonic/gin"
)

type responseBody struct {
	ErrCode int             `json:"err_code"`
	Msg     string          `json:"msg"`
	Data    json.RawMessage `json:"data"`
}

func TestSuccessUsesHTTP200ErrCode(t *testing.T) {
	w, c := testContext()

	Success(c, gin.H{"id": 1})

	body := decodeBody(t, w)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if body.ErrCode != SuccessCode {
		t.Fatalf("err_code = %d, want %d", body.ErrCode, SuccessCode)
	}
	if body.ErrCode != http.StatusOK {
		t.Fatalf("success err_code = %d, want %d", body.ErrCode, http.StatusOK)
	}
	if body.Msg != SuccessMsg {
		t.Fatalf("msg = %q, want %q", body.Msg, SuccessMsg)
	}
}

func TestErrorMapsAppError(t *testing.T) {
	w, c := testContext()
	err := apperrors.WrapWithData(
		stderrors.New("permission denied"),
		apperrors.CodeForbidden,
		apperrors.SubCodeForbidden,
		"无权访问",
		gin.H{"scope": "admin"},
	)

	Error(c, err)

	body := decodeBody(t, w)
	if body.ErrCode != apperrors.CodeForbidden {
		t.Fatalf("err_code = %d, want %d", body.ErrCode, apperrors.CodeForbidden)
	}
	if body.Msg != "无权访问" {
		t.Fatalf("msg = %q", body.Msg)
	}
	if string(body.Data) != `{"scope":"admin"}` {
		t.Fatalf("unexpected data: %s", body.Data)
	}
}

func TestFailMapsAppError(t *testing.T) {
	w, c := testContext()

	Fail(c, apperrors.ErrNotFound)

	body := decodeBody(t, w)
	if body.ErrCode != apperrors.CodeNotFound {
		t.Fatalf("err_code = %d, want %d", body.ErrCode, apperrors.CodeNotFound)
	}
}

func TestErrorMapsGenericErrorToInternal(t *testing.T) {
	w, c := testContext()

	Error(c, stderrors.New("boom"))

	body := decodeBody(t, w)
	if body.ErrCode != apperrors.CodeInternal {
		t.Fatalf("err_code = %d, want %d", body.ErrCode, apperrors.CodeInternal)
	}
	if body.Msg != apperrors.ErrInternalServer.Message {
		t.Fatalf("msg = %q, want %q", body.Msg, apperrors.ErrInternalServer.Message)
	}
}

func testContext() (*httptest.ResponseRecorder, *gin.Context) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	return w, c
}

func decodeBody(t *testing.T, w *httptest.ResponseRecorder) responseBody {
	t.Helper()
	var body responseBody
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v body=%s", err, w.Body.String())
	}
	return body
}
