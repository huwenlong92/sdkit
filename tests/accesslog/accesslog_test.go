package accesslog_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/huwenlong92/sdkit/core/gin/accesslog"
	ginrequestid "github.com/huwenlong92/sdkit/core/gin/requestid"
	gintracing "github.com/huwenlong92/sdkit/core/gin/tracing"
	gintracking "github.com/huwenlong92/sdkit/core/gin/tracking"
	"github.com/huwenlong92/sdkit/core/logger"
	"github.com/huwenlong92/sdkit/core/requestid"
	"github.com/huwenlong92/sdkit/core/tracking"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
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

func TestFilterHeadersWithSensitiveHeaders(t *testing.T) {
	h := http.Header{}
	h.Set("X-Internal-Secret", "secret")
	h.Set("Authorization", "Bearer token")
	h.Set("X-Visible", "visible")

	result := accesslog.FilterHeadersWithSensitiveHeaders(h, "X-Internal-Secret")

	if strings.Contains(result, "X-Internal-Secret") || strings.Contains(result, "secret") {
		t.Fatalf("custom sensitive header should be filtered: %s", result)
	}
	if !strings.Contains(result, "Authorization") || !strings.Contains(result, "Bearer token") {
		t.Fatalf("default sensitive header should be kept when custom list replaces defaults: %s", result)
	}
	if !strings.Contains(result, "X-Visible") || !strings.Contains(result, "visible") {
		t.Fatalf("visible header should be kept: %s", result)
	}
}

func TestFilterHeadersWithAdditionalSensitiveHeaders(t *testing.T) {
	h := http.Header{}
	h.Set("Authorization", "Bearer token")
	h.Set("X-Internal-Secret", "secret")
	h.Set("X-Visible", "visible")

	result := accesslog.FilterHeadersWithAdditionalSensitiveHeaders(h, "X-Internal-Secret")

	if strings.Contains(result, "Authorization") || strings.Contains(result, "Bearer token") {
		t.Fatalf("default sensitive header should be filtered: %s", result)
	}
	if strings.Contains(result, "X-Internal-Secret") || strings.Contains(result, "secret") {
		t.Fatalf("additional sensitive header should be filtered: %s", result)
	}
	if !strings.Contains(result, "X-Visible") || !strings.Contains(result, "visible") {
		t.Fatalf("visible header should be kept: %s", result)
	}
}

func TestGetRequestQuery(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/test?a=1&b=2&c=3", nil)

	result := accesslog.GetRequestQuery(c)

	if result["a"] != "1" || result["b"] != "2" || result["c"] != "3" {
		t.Fatalf("wrong query: %v", result)
	}
}

func TestGetRequestQueryEmpty(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/test", nil)

	result := accesslog.GetRequestQuery(c)
	if len(result) != 0 {
		t.Fatalf("expected empty, got %v", result)
	}
}

func TestGetRequestBodyJSON(t *testing.T) {
	w := httptest.NewRecorder()
	body := `{"name":"test","age":30}`
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(body))
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
	c.Request = httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(form.Encode()))
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
	c.Request = httptest.NewRequest(http.MethodGet, "/test", nil)
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
	c.Request = httptest.NewRequest(http.MethodPost, "/test?page=1&size=10", strings.NewReader(form.Encode()))
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
	c.Request = httptest.NewRequest(http.MethodGet, "/test?a=1&b=2", nil)

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

func TestMiddlewareWithoutLoggerPassesThrough(t *testing.T) {
	r := gin.New()
	r.Use(accesslog.Middleware("test"))
	r.POST("/body", func(c *gin.Context) {
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"err": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"size": len(body)})
	})

	body := strings.Repeat("a", 1<<20+2048)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/body", strings.NewReader(body))
	req.Header.Set("Content-Type", "text/plain")

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"size":1050624`) {
		t.Fatalf("handler should receive full body, got response %s", w.Body.String())
	}
}

func TestMiddlewareWithSkipperSkipsAccessLog(t *testing.T) {
	writer, accessLogger, stop := newCaptureLogger(t)
	defer stop()

	r := gin.New()
	r.Use(accesslog.Middleware("test", accesslog.WithLogger(accessLogger), accesslog.WithSkipper(func(c *gin.Context) bool {
		return c.FullPath() == "/skip"
	})))
	r.GET("/skip", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/skip", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d", w.Code)
	}
	assertNoEntry(t, writer.ch)
}

func TestMiddlewareWithSkipMethodsSkipsAccessLog(t *testing.T) {
	writer, accessLogger, stop := newCaptureLogger(t)
	defer stop()

	r := gin.New()
	r.Use(accesslog.Middleware("test", accesslog.WithLogger(accessLogger), accesslog.WithSkipMethods("options", "TRACE")))
	r.Any("/method", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/method", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d", w.Code)
	}
	assertNoEntry(t, writer.ch)

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/method", nil)
	r.ServeHTTP(w, req)

	entry := waitEntry(t, writer.ch)
	if entry.Method != http.MethodGet {
		t.Fatalf("method = %s, want GET", entry.Method)
	}
}

func TestMiddlewareWithSkipIPsSkipsAccessLog(t *testing.T) {
	writer, accessLogger, stop := newCaptureLogger(t)
	defer stop()

	r := gin.New()
	r.Use(accesslog.Middleware("test", accesslog.WithLogger(accessLogger), accesslog.WithSkipIPs("192.168.1.10", "10.0.0.0/8")))
	r.GET("/ip", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ip", nil)
	req.RemoteAddr = "192.168.1.10:1234"
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d", w.Code)
	}
	assertNoEntry(t, writer.ch)

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/ip", nil)
	req.RemoteAddr = "10.2.3.4:1234"
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d", w.Code)
	}
	assertNoEntry(t, writer.ch)

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/ip", nil)
	req.RemoteAddr = "172.16.0.1:1234"
	r.ServeHTTP(w, req)

	entry := waitEntry(t, writer.ch)
	if entry.IP != "172.16.0.1" {
		t.Fatalf("ip = %s, want 172.16.0.1", entry.IP)
	}
}

func TestMiddlewareSkipSkipsAccessLogAfterHandler(t *testing.T) {
	writer, accessLogger, stop := newCaptureLogger(t)
	defer stop()

	r := gin.New()
	r.Use(accesslog.Middleware("test", accesslog.WithLogger(accessLogger)))
	r.GET("/skip", func(c *gin.Context) {
		accesslog.Skip(c)
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/skip", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d", w.Code)
	}
	assertNoEntry(t, writer.ch)
}

func TestMiddlewareRedactsJSONSensitiveFields(t *testing.T) {
	writer, accessLogger, stop := newCaptureLogger(t)
	defer stop()

	r := gin.New()
	r.Use(accesslog.Middleware("test", accesslog.WithLogger(accessLogger)))
	r.POST("/login", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	body := `{"username":"admin","password":"secret","profile":{"access_token":"token-value"},"sessions":[{"client_secret":"client-secret"}]}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	r.ServeHTTP(w, req)

	entry := waitEntry(t, writer.ch)
	reqBody := string(entry.ReqBody)
	if !strings.Contains(reqBody, `"password":"(redacted)"`) {
		t.Fatalf("password should be redacted: %s", reqBody)
	}
	if !strings.Contains(reqBody, `"access_token":"(redacted)"`) {
		t.Fatalf("nested token should be redacted: %s", reqBody)
	}
	if !strings.Contains(reqBody, `"client_secret":"(redacted)"`) {
		t.Fatalf("nested secret should be redacted: %s", reqBody)
	}
	if strings.Contains(reqBody, `"password":"secret"`) ||
		strings.Contains(reqBody, "token-value") ||
		strings.Contains(reqBody, "client-secret") {
		t.Fatalf("sensitive values should not remain: %s", reqBody)
	}
}

func TestMiddlewareRedactsConfiguredFieldsAndHeaders(t *testing.T) {
	writer, accessLogger, stop := newCaptureLogger(t)
	defer stop()

	r := gin.New()
	r.Use(accesslog.Middleware(
		"test",
		accesslog.WithLogger(accessLogger),
		accesslog.WithAdditionalSensitiveFields("otp", "pin_code"),
		accesslog.WithAdditionalSensitiveHeaders("X-Internal-Secret"),
	))
	r.POST("/verify", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	body := `{"username":"admin","otp":"123456","nested":{"pin_code":"9999"}}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/verify", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Secret", "secret-value")
	req.Header.Set("X-Request-From", "test")

	r.ServeHTTP(w, req)

	entry := waitEntry(t, writer.ch)
	reqBody := string(entry.ReqBody)
	if !strings.Contains(reqBody, `"otp":"(redacted)"`) {
		t.Fatalf("configured otp field should be redacted: %s", reqBody)
	}
	if !strings.Contains(reqBody, `"pin_code":"(redacted)"`) {
		t.Fatalf("configured nested pin_code field should be redacted: %s", reqBody)
	}
	if strings.Contains(reqBody, "123456") || strings.Contains(reqBody, "9999") {
		t.Fatalf("configured sensitive values should not remain: %s", reqBody)
	}

	headers := string(entry.Headers)
	if strings.Contains(headers, "X-Internal-Secret") || strings.Contains(headers, "secret-value") {
		t.Fatalf("configured sensitive header should be filtered: %s", headers)
	}
	if !strings.Contains(headers, "X-Request-From") {
		t.Fatalf("non-sensitive header should be kept: %s", headers)
	}
}

func TestMiddlewareAllowsEmptySensitiveFieldsAndHeaders(t *testing.T) {
	writer, accessLogger, stop := newCaptureLogger(t)
	defer stop()

	r := gin.New()
	r.Use(accesslog.Middleware(
		"test",
		accesslog.WithLogger(accessLogger),
		accesslog.WithSensitiveFields(),
		accesslog.WithSensitiveHeaders(),
	))
	r.POST("/debug", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	body := `{"username":"admin","password":"secret","token":"token-value"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/debug", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer token")

	r.ServeHTTP(w, req)

	entry := waitEntry(t, writer.ch)
	reqBody := string(entry.ReqBody)
	if !strings.Contains(reqBody, `"password":"secret"`) || !strings.Contains(reqBody, `"token":"token-value"`) {
		t.Fatalf("empty sensitive fields should keep original body fields: %s", reqBody)
	}

	headers := string(entry.Headers)
	if !strings.Contains(headers, "Authorization") || !strings.Contains(headers, "Bearer token") {
		t.Fatalf("empty sensitive headers should keep original headers: %s", headers)
	}
}

func TestMiddlewareRedactsFormBodyAndPreservesHandlerInput(t *testing.T) {
	writer, accessLogger, stop := newCaptureLogger(t)
	defer stop()

	r := gin.New()
	r.Use(accesslog.Middleware("test", accesslog.WithLogger(accessLogger)))
	r.POST("/login", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"password": c.PostForm("password")})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader("username=admin&password=123"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	r.ServeHTTP(w, req)

	if !strings.Contains(w.Body.String(), `"password":"123"`) {
		t.Fatalf("handler should receive original form body, got %s", w.Body.String())
	}
	entry := waitEntry(t, writer.ch)
	reqBody := string(entry.ReqBody)
	if !strings.Contains(reqBody, `"username":"admin"`) {
		t.Fatalf("username should be kept: %s", reqBody)
	}
	if !strings.Contains(reqBody, `"password":"(redacted)"`) {
		t.Fatalf("password should be redacted: %s", reqBody)
	}
	if strings.Contains(reqBody, "123") {
		t.Fatalf("sensitive form value should not remain: %s", reqBody)
	}
}

func TestMiddlewareRedactsMultipartFields(t *testing.T) {
	writer, accessLogger, stop := newCaptureLogger(t)
	defer stop()

	r := gin.New()
	r.Use(accesslog.Middleware("test", accesslog.WithLogger(accessLogger)))
	r.POST("/upload", func(c *gin.Context) {
		file, err := c.FormFile("file")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"err": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"file":     file.Filename,
			"password": c.PostForm("password"),
		})
	})

	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	if err := mw.WriteField("title", "hello"); err != nil {
		t.Fatal(err)
	}
	if err := mw.WriteField("password", "123"); err != nil {
		t.Fatal(err)
	}
	part, err := mw.CreateFormFile("file", "test.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("file content")); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/upload", body)
	req.Header.Set("Content-Type", mw.FormDataContentType())

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"password":"123"`) {
		t.Fatalf("handler should receive original multipart fields, got %s", w.Body.String())
	}
	entry := waitEntry(t, writer.ch)
	reqBody := string(entry.ReqBody)
	if !strings.Contains(reqBody, `"title":"hello"`) {
		t.Fatalf("title should be kept: %s", reqBody)
	}
	if !strings.Contains(reqBody, `"password":"(redacted)"`) {
		t.Fatalf("password should be redacted: %s", reqBody)
	}
	if strings.Contains(reqBody, "file content") || strings.Contains(reqBody, "123") {
		t.Fatalf("multipart log should not contain file content or secret: %s", reqBody)
	}
}

func TestMiddlewarePreservesLargeBodyAfterSampling(t *testing.T) {
	writer, accessLogger, stop := newCaptureLogger(t)
	defer stop()

	r := gin.New()
	r.Use(accesslog.Middleware("test", accesslog.WithLogger(accessLogger)))
	r.POST("/large", func(c *gin.Context) {
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"err": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"size": len(body)})
	})

	body := strings.Repeat("a", 1<<20+2048)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/large", strings.NewReader(body))
	req.Header.Set("Content-Type", "text/plain")

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"size":1050624`) {
		t.Fatalf("handler should receive full body, got response %s", w.Body.String())
	}
	entry := waitEntry(t, writer.ch)
	reqBody := string(entry.ReqBody)
	if !strings.HasSuffix(reqBody, "...(truncated)") {
		t.Fatalf("large body log should be truncated, got suffix from %q", reqBody[len(reqBody)-32:])
	}
	if len(reqBody) >= len(body) {
		t.Fatalf("log body should be sampled, got log=%d original=%d", len(reqBody), len(body))
	}
}

func TestMiddlewareBinaryResponseOmitted(t *testing.T) {
	writer, accessLogger, stop := newCaptureLogger(t)
	defer stop()

	r := gin.New()
	r.Use(accesslog.Middleware("test", accesslog.WithLogger(accessLogger)))
	r.GET("/download", func(c *gin.Context) {
		c.Data(http.StatusOK, "application/octet-stream", bytes.Repeat([]byte{0, 1, 2, 3}, 1000))
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/download", nil)

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d", w.Code)
	}
	if len(w.Body.Bytes()) != 4000 {
		t.Fatalf("response should be complete: want 4000, got %d", len(w.Body.Bytes()))
	}
	entry := waitEntry(t, writer.ch)
	if string(entry.RespBody) != "(binary body omitted)" {
		t.Fatalf("binary response should be omitted, got %q", string(entry.RespBody))
	}
}

func TestMiddlewareExtractsResponseMeta(t *testing.T) {
	writer, accessLogger, stop := newCaptureLogger(t)
	defer stop()

	r := gin.New()
	r.Use(accesslog.Middleware("test", accesslog.WithLogger(accessLogger)))
	r.GET("/ok", func(c *gin.Context) {
		c.JSON(http.StatusOK, struct {
			ErrCode int    `json:"err_code"`
			Msg     string `json:"msg"`
			Data    string `json:"data"`
		}{
			ErrCode: 200,
			Msg:     "ok",
			Data:    "hello",
		})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ok", nil)
	r.ServeHTTP(w, req)

	entry := waitEntry(t, writer.ch)
	if entry.ErrCode != 200 {
		t.Fatalf("err_code = %d, want 200", entry.ErrCode)
	}
	if entry.ErrMsg != "ok" {
		t.Fatalf("err_msg = %q, want ok", entry.ErrMsg)
	}
}

func TestMiddlewareExtractsResponseMetaFromTruncatedBody(t *testing.T) {
	writer, accessLogger, stop := newCaptureLogger(t)
	defer stop()

	r := gin.New()
	r.Use(accesslog.Middleware("test", accesslog.WithLogger(accessLogger)))
	r.GET("/large", func(c *gin.Context) {
		c.Data(http.StatusOK, "application/json", []byte(`{"err_code":200,"msg":"ok","data":"`+strings.Repeat("a", 64*1024)+`"}`))
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/large", nil)
	r.ServeHTTP(w, req)

	entry := waitEntry(t, writer.ch)
	if !strings.HasSuffix(string(entry.RespBody), "...(truncated)") {
		t.Fatalf("response body should be truncated")
	}
	if entry.ErrCode != 200 {
		t.Fatalf("err_code = %d, want 200", entry.ErrCode)
	}
	if entry.ErrMsg != "ok" {
		t.Fatalf("err_msg = %q, want ok", entry.ErrMsg)
	}
}

func TestMiddlewareSeparatesTrackIDAndTraceID(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithSpanProcessor(recorder),
	)
	oldProvider := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	defer otel.SetTracerProvider(oldProvider)
	defer provider.Shutdown(context.Background())

	writer, accessLogger, stop := newCaptureLogger(t)
	defer stop()

	r := gin.New()
	r.Use(gintracking.Middleware())
	r.Use(gintracing.Middleware("accesslog-test"))
	r.Use(ginrequestid.Middleware())
	r.Use(accesslog.Middleware("test", accesslog.WithLogger(accessLogger)))
	r.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set(tracking.Header, "track-accesslog")
	req.Header.Set(requestid.Header, "request-accesslog")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	entry := waitEntry(t, writer.ch)
	if entry.TrackID != "track-accesslog" {
		t.Fatalf("track_id = %q, want track-accesslog", entry.TrackID)
	}
	if entry.RequestID != "request-accesslog" {
		t.Fatalf("request_id = %q, want request-accesslog", entry.RequestID)
	}
	if entry.TraceID == "" {
		t.Fatal("trace_id should not be empty when tracing middleware is registered")
	}
	if entry.TraceID == entry.TrackID {
		t.Fatalf("trace_id should not reuse track_id: trace_id=%q track_id=%q", entry.TraceID, entry.TrackID)
	}
}

type captureWriter struct {
	ch chan *accesslog.Entry
}

func newCaptureLogger(t *testing.T) (*captureWriter, *accesslog.Logger, context.CancelFunc) {
	t.Helper()

	writer := &captureWriter{ch: make(chan *accesslog.Entry, 1)}
	logCtx, stop := context.WithCancel(context.Background())
	accessLogger := accesslog.NewLogger(writer, accesslog.Config{
		BatchSize:     1,
		FlushInterval: time.Hour,
	})
	accessLogger.Start(logCtx)
	return writer, accessLogger, stop
}

func (w *captureWriter) WriteBatch(ctx context.Context, entries []*accesslog.Entry) error {
	for _, entry := range entries {
		w.ch <- entry
	}
	return nil
}

func waitEntry(t *testing.T, ch <-chan *accesslog.Entry) *accesslog.Entry {
	t.Helper()
	select {
	case entry := <-ch:
		return entry
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for accesslog entry")
		return nil
	}
}

func assertNoEntry(t *testing.T, ch <-chan *accesslog.Entry) {
	t.Helper()
	select {
	case entry := <-ch:
		t.Fatalf("unexpected accesslog entry: method=%s path=%s", entry.Method, entry.Path)
	case <-time.After(100 * time.Millisecond):
	}
}
