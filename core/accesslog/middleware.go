package accesslog

import (
	"bytes"
	"fmt"
	"io"
	"mime"
	"strings"
	"time"

	"github.com/huwenlong92/sdkit/core/jsonx"
	"github.com/huwenlong92/sdkit/core/requestid"
	"github.com/huwenlong92/sdkit/core/tracing"
	"github.com/huwenlong92/sdkit/core/tracking"

	"github.com/gin-gonic/gin"
)

const maxBodySize = 32 * 1024

type Actor struct {
	ID   string
	Type string
	Name string
}

type ActorResolver func(*gin.Context) Actor

type middlewareConfig struct {
	logger        *Logger
	actorResolver ActorResolver
}

type MiddlewareOption func(*middlewareConfig)

func WithLogger(logger *Logger) MiddlewareOption {
	return func(cfg *middlewareConfig) {
		cfg.logger = logger
	}
}

func WithActorResolver(resolver ActorResolver) MiddlewareOption {
	return func(cfg *middlewareConfig) {
		cfg.actorResolver = resolver
	}
}

type bodyWriter struct {
	gin.ResponseWriter
	buf    *bytes.Buffer
	maxCap int
}

func (w *bodyWriter) Write(b []byte) (int, error) {
	if w.buf.Len() < w.maxCap {
		remain := w.maxCap - w.buf.Len()
		if len(b) > remain {
			w.buf.Write(b[:remain])
		} else {
			w.buf.Write(b)
		}
	}
	return w.ResponseWriter.Write(b)
}

func (w *bodyWriter) WriteString(s string) (int, error) {
	if w.buf.Len() < w.maxCap {
		remain := w.maxCap - w.buf.Len()
		if len(s) > remain {
			w.buf.WriteString(s[:remain])
		} else {
			w.buf.WriteString(s)
		}
	}
	return w.ResponseWriter.WriteString(s)
}

// Middleware records HTTP request and response metadata.
// The logger is optional so services can opt in with WithLogger.
func Middleware(source string, opts ...MiddlewareOption) gin.HandlerFunc {
	cfg := &middlewareConfig{}
	for _, opt := range opts {
		if opt != nil {
			opt(cfg)
		}
	}

	return func(c *gin.Context) {
		start := time.Now()

		reqBody := captureReqBody(c)

		bw := &bodyWriter{
			ResponseWriter: c.Writer,
			buf:            bytes.NewBuffer(nil),
			maxCap:         maxBodySize,
		}
		c.Writer = bw

		c.Next()

		entry := &Entry{
			Source:     source,
			TrackID:    tracking.Get(c),
			TraceID:    tracing.TraceID(c.Request.Context()),
			RequestID:  requestid.Get(c),
			UID:        resolveUID(c, cfg),
			Method:     c.Request.Method,
			Path:       c.Request.URL.Path,
			Query:      c.Request.URL.RawQuery,
			IP:         c.ClientIP(),
			UserAgent:  c.Request.UserAgent(),
			Headers:    []byte(FilterHeaders(c.Request.Header)),
			ReqBody:    reqBody,
			StatusCode: c.Writer.Status(),
			RespBody:   respBodyBytes(c, bw),
			Latency:    time.Since(start).Milliseconds(),
			CreatedAt:  time.Now().UnixMilli(),
		}
		cfg.logger.Push(entry)
	}
}

// maxReadBody is the upper bound for reading request bodies into memory.
const maxReadBody = 1 << 20 // 1MB

func captureReqBody(c *gin.Context) []byte {
	if c.Request.Body == nil {
		return nil
	}
	contentType := c.Request.Header.Get("Content-Type")

	if strings.Contains(contentType, "multipart/form-data") {
		fields, _ := GetRequestBody(c)
		if len(fields) > 0 {
			b, _ := jsonx.Marshal(fields)
			return b
		}
		return nil
	}

	if isBinContentType(contentType) {
		return []byte("(binary body omitted)")
	}

	bodyBytes, _ := io.ReadAll(io.LimitReader(c.Request.Body, maxReadBody))
	c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	if strings.Contains(contentType, "application/json") && len(bodyBytes) > 0 {
		if summarized := summarizeJSON(bodyBytes); len(summarized) > 0 {
			return summarized
		}
	}

	if len(bodyBytes) > maxBodySize {
		return append(bodyBytes[:maxBodySize], []byte("...(truncated)")...)
	}
	return bodyBytes
}

func isBinContentType(contentType string) bool {
	if contentType == "" {
		return false
	}
	mediaType, _, _ := mime.ParseMediaType(contentType)
	return strings.HasPrefix(mediaType, "image/") ||
		strings.HasPrefix(mediaType, "video/") ||
		strings.HasPrefix(mediaType, "audio/") ||
		mediaType == "application/octet-stream"
}

func respBodyBytes(c *gin.Context, bw *bodyWriter) []byte {
	contentType := c.Writer.Header().Get("Content-Type")
	if contentType == "" {
		contentType = "text/plain"
	}

	mediaType, _, _ := mime.ParseMediaType(contentType)
	if !isTextMedia(mediaType) {
		return []byte("(binary body omitted)")
	}

	respBody := bw.buf.Bytes()
	if len(respBody) >= maxBodySize {
		out := make([]byte, 0, len(respBody)+14)
		out = append(out, respBody...)
		out = append(out, []byte("...(truncated)")...)
		return out
	}
	out := make([]byte, len(respBody))
	copy(out, respBody)
	return out
}

func isTextMedia(mediaType string) bool {
	if strings.HasPrefix(mediaType, "text/") {
		return true
	}
	switch mediaType {
	case "application/json", "application/xml", "application/javascript",
		"application/x-www-form-urlencoded":
		return true
	}
	return false
}

func resolveUID(c *gin.Context, cfg *middlewareConfig) string {
	if cfg != nil && cfg.actorResolver != nil {
		actor := cfg.actorResolver(c)
		return actor.ID
	}
	if uid := c.GetString("uid"); uid != "" {
		return uid
	}
	return ""
}

const maxFieldLen = 200

// summarizeJSON replaces large string fields with a short placeholder.
func summarizeJSON(body []byte) []byte {
	var data map[string]interface{}
	if err := jsonx.Unmarshal(body, &data); err != nil {
		return nil
	}
	walkAndSummarize(data)
	b, _ := jsonx.Marshal(data)
	return b
}

func walkAndSummarize(data map[string]interface{}) {
	for k, v := range data {
		if isSensitiveField(k) {
			data[k] = "(redacted)"
			continue
		}
		switch val := v.(type) {
		case string:
			if len(val) > maxFieldLen {
				data[k] = fmt.Sprintf("(string: %d chars)", len(val))
			}
		case map[string]interface{}:
			walkAndSummarize(val)
		case []interface{}:
			walkAndSummarizeSlice(val)
		}
	}
}

func walkAndSummarizeSlice(values []interface{}) {
	for _, v := range values {
		switch val := v.(type) {
		case map[string]interface{}:
			walkAndSummarize(val)
		case []interface{}:
			walkAndSummarizeSlice(val)
		}
	}
}

func isSensitiveField(key string) bool {
	key = strings.ToLower(key)
	return strings.Contains(key, "authorization") ||
		strings.Contains(key, "cookie") ||
		strings.Contains(key, "password") ||
		strings.Contains(key, "token") ||
		strings.Contains(key, "secret")
}
