package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/huwenlong92/sdkit/core/accesslog"
	"github.com/huwenlong92/sdkit/core/logger"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	logger.L = zap.NewNop()
	os.Exit(m.Run())
}

func TestFilterHeaders(t *testing.T) {
	h := http.Header{}
	h.Set("Authorization", "Bearer xxx")
	h.Set("Content-Type", "application/json")
	h.Set("X-Api-Key", "secret")
	h.Set("Cookie", "session=abc")
	h.Set("Accept", "text/html")

	result := accesslog.FilterHeaders(h)

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatal("FilterHeaders should return valid JSON:", err)
	}

	if _, ok := parsed["Authorization"]; ok {
		t.Fatal("Authorization should be filtered")
	}
	if _, ok := parsed["Cookie"]; ok {
		t.Fatal("Cookie should be filtered")
	}
	if _, ok := parsed["X-Api-Key"]; ok {
		t.Fatal("X-Api-Key should be filtered")
	}
	if _, ok := parsed["Content-Type"]; !ok {
		t.Fatal("Content-Type should be kept")
	}
	if _, ok := parsed["Accept"]; !ok {
		t.Fatal("Accept should be kept")
	}
}

func TestFilterHeadersEmpty(t *testing.T) {
	if s := accesslog.FilterHeaders(http.Header{}); s != "" {
		t.Fatal("empty headers should return empty string")
	}
}

func TestGetRequestQuery(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/test?a=1&b=2&c=3", nil)

	result := accesslog.GetRequestQuery(c)

	if result["a"] != "1" || result["b"] != "2" || result["c"] != "3" {
		t.Fatalf("wrong query: %v", result)
	}
}

func TestGetRequestQueryEmpty(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/test", nil)

	result := accesslog.GetRequestQuery(c)
	if len(result) != 0 {
		t.Fatalf("expected empty, got %v", result)
	}
}

func TestGetRequestBodyJSON(t *testing.T) {
	w := httptest.NewRecorder()
	body := `{"name":"test","age":30}`
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/test", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	result, err := accesslog.GetRequestBody(c)
	if err != nil {
		t.Fatal(err)
	}
	if result["name"] != "test" {
		t.Fatalf("name: want test, got %v", result["name"])
	}
	if result["age"] != float64(30) {
		t.Fatalf("age: want 30, got %v", result["age"])
	}
}

func TestGetRequestBodyForm(t *testing.T) {
	w := httptest.NewRecorder()
	form := url.Values{}
	form.Set("username", "admin")
	form.Set("password", "123456")

	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/test", strings.NewReader(form.Encode()))
	c.Request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	result, err := accesslog.GetRequestBody(c)
	if err != nil {
		t.Fatal(err)
	}
	if result["username"] != "admin" {
		t.Fatalf("username: want admin, got %v", result["username"])
	}
	if result["password"] != "123456" {
		t.Fatalf("password: want 123456, got %v", result["password"])
	}
}

func TestGetRequestHeaders(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/test", nil)
	c.Request.Header.Set("X-Custom", "value1")
	c.Request.Header.Set("Accept", "application/json")

	result := accesslog.GetRequestHeaders(c)
	if result["X-Custom"] != "value1" {
		t.Fatalf("X-Custom: want value1, got %v", result["X-Custom"])
	}
	if result["Accept"] != "application/json" {
		t.Fatalf("Accept mismatch: %v", result["Accept"])
	}
}

func TestRequestInputs(t *testing.T) {
	w := httptest.NewRecorder()
	form := url.Values{}
	form.Set("password", "secret")

	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/test?page=1&size=10", strings.NewReader(form.Encode()))
	c.Request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	result, err := accesslog.RequestInputs(c)
	if err != nil {
		t.Fatal(err)
	}

	getParams := result["GET"].(map[string]interface{})
	postParams := result["POST"].(map[string]interface{})

	if getParams["page"] != "1" {
		t.Fatalf("GET.page: want 1, got %v", getParams["page"])
	}
	if getParams["size"] != "10" {
		t.Fatalf("GET.size: want 10, got %v", getParams["size"])
	}
	if postParams["password"] != "secret" {
		t.Fatalf("POST.password: want secret, got %v", postParams["password"])
	}
}

func TestRequestInputsGET(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/test?a=1&b=2", nil)

	result, err := accesslog.RequestInputs(c)
	if err != nil {
		t.Fatal(err)
	}

	getParams := result["GET"].(map[string]interface{})
	if len(getParams) != 2 {
		t.Fatalf("GET params: expected 2, got %d", len(getParams))
	}
	if getParams["a"] != "1" {
		t.Fatalf("GET.a: want 1, got %v", getParams["a"])
	}
	if _, ok := result["POST"]; ok {
		t.Fatal("POST should not exist for GET request")
	}
}

func TestMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(accesslog.Middleware("test"))
	r.POST("/api/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	body := `{"action":"ping"}`
	req := httptest.NewRequest("POST", "/api/test?t=1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer secret123")

	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status: want 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["ok"] != true {
		t.Fatal("response body should be untouched")
	}
}

func TestMiddlewareBodyTruncated(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(accesslog.Middleware("test"))
	r.GET("/big", func(c *gin.Context) {
		big := bytes.Repeat([]byte("x"), 32*1024+100)
		c.String(200, string(big))
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/big", nil)

	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status: want 200, got %d", w.Code)
	}
	if len(w.Body.String()) != 32*1024+100 {
		t.Fatalf("response should be complete: want %d, got %d", 32*1024+100, len(w.Body.String()))
	}
}

func TestMiddlewareBinaryResp(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(accesslog.Middleware("test"))
	r.GET("/download", func(c *gin.Context) {
		c.Header("Content-Type", "application/octet-stream")
		c.Header("Content-Disposition", "attachment; filename=test.bin")
		c.Data(200, "application/octet-stream", bytes.Repeat([]byte{0, 1, 2, 3}, 1000))
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/download", nil)

	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status: want 200, got %d", w.Code)
	}
	if len(w.Body.Bytes()) != 4000 {
		t.Fatalf("response should be complete: want 4000, got %d", len(w.Body.Bytes()))
	}
}

func TestMiddlewareImageResp(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(accesslog.Middleware("test"))
	r.GET("/image", func(c *gin.Context) {
		c.Data(200, "image/png", bytes.Repeat([]byte{0x89, 0x50}, 512))
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/image", nil)

	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status: want 200, got %d", w.Code)
	}
	if len(w.Body.Bytes()) != 1024 {
		t.Fatalf("response should be complete: want 1024, got %d", len(w.Body.Bytes()))
	}
}
