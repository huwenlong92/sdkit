package accesslog

import (
	"bytes"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/netip"
	"net/url"
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
type Skipper func(*gin.Context) bool

type middlewareConfig struct {
	logger                 *Logger
	actorResolver          ActorResolver
	skipper                Skipper
	skipMethods            map[string]struct{}
	skipIPAddrs            map[netip.Addr]struct{}
	skipIPPrefixes         []netip.Prefix
	sensitiveFieldKeywords []string
	sensitiveHeaders       []string
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

func WithSkipper(skipper Skipper) MiddlewareOption {
	return func(cfg *middlewareConfig) {
		cfg.skipper = skipper
	}
}

// WithSkipMethods skips access logging for matched HTTP methods.
func WithSkipMethods(methods ...string) MiddlewareOption {
	return func(cfg *middlewareConfig) {
		cfg.skipMethods = normalizeSkipMethods(methods...)
	}
}

// WithSkipIPs skips access logging for matched client IPs.
// Values support exact IPs such as "127.0.0.1" and CIDR ranges such as "10.0.0.0/8".
func WithSkipIPs(values ...string) MiddlewareOption {
	return func(cfg *middlewareConfig) {
		cfg.skipIPAddrs, cfg.skipIPPrefixes = normalizeSkipIPs(values...)
	}
}

// WithSensitiveFields replaces body field redaction keywords.
// Passing no fields disables body field redaction for this middleware.
func WithSensitiveFields(fields ...string) MiddlewareOption {
	return func(cfg *middlewareConfig) {
		cfg.sensitiveFieldKeywords = normalizeSensitiveValues(fields...)
	}
}

// WithAdditionalSensitiveFields appends body field keywords to the default redaction list.
func WithAdditionalSensitiveFields(fields ...string) MiddlewareOption {
	return func(cfg *middlewareConfig) {
		cfg.sensitiveFieldKeywords = appendSensitiveValues(cfg.sensitiveFieldKeywords, fields...)
	}
}

// WithSensitiveHeaders replaces request header filter names.
// Passing no headers disables header filtering for this middleware.
func WithSensitiveHeaders(headers ...string) MiddlewareOption {
	return func(cfg *middlewareConfig) {
		cfg.sensitiveHeaders = normalizeSensitiveValues(headers...)
	}
}

// WithAdditionalSensitiveHeaders appends request header names to the default filter list.
func WithAdditionalSensitiveHeaders(headers ...string) MiddlewareOption {
	return func(cfg *middlewareConfig) {
		cfg.sensitiveHeaders = appendSensitiveValues(cfg.sensitiveHeaders, headers...)
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

type replayBody struct {
	io.Reader
	closer io.Closer
}

func (b replayBody) Close() error {
	if b.closer == nil {
		return nil
	}
	return b.closer.Close()
}

const skipKey = "__sdkit_accesslog_skip__"

// Skip marks current Gin request as skipped by accesslog middleware.
func Skip(c *gin.Context) {
	if c == nil {
		return
	}
	c.Set(skipKey, true)
}

// IsSkipped reports whether current Gin request is marked to skip access logging.
func IsSkipped(c *gin.Context) bool {
	if c == nil {
		return false
	}
	value, ok := c.Get(skipKey)
	if !ok {
		return false
	}
	skipped, ok := value.(bool)
	return ok && skipped
}

// Middleware records HTTP request and response metadata.
// The logger is optional so services can opt in with WithLogger.
func Middleware(source string, opts ...MiddlewareOption) gin.HandlerFunc {
	cfg := defaultMiddlewareConfig()
	for _, opt := range opts {
		if opt != nil {
			opt(cfg)
		}
	}
	if cfg.logger == nil {
		return func(c *gin.Context) {
			c.Next()
		}
	}

	return func(c *gin.Context) {
		if shouldSkip(c, cfg) {
			c.Next()
			return
		}
		start := time.Now()

		reqBody := captureReqBody(c, cfg)

		bw := &bodyWriter{
			ResponseWriter: c.Writer,
			buf:            bytes.NewBuffer(nil),
			maxCap:         maxBodySize,
		}
		c.Writer = bw

		c.Next()

		if shouldSkip(c, cfg) {
			return
		}
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
			Headers:    []byte(filterHeaders(c.Request.Header, cfg.sensitiveHeaders)),
			ReqBody:    reqBody,
			StatusCode: c.Writer.Status(),
			RespBody:   respBodyBytes(c, bw),
			Latency:    time.Since(start).Milliseconds(),
			CreatedAt:  time.Now().UnixMilli(),
		}
		cfg.logger.Push(entry)
	}
}

func shouldSkip(c *gin.Context, cfg *middlewareConfig) bool {
	if IsSkipped(c) {
		return true
	}
	if cfg != nil && cfg.skipper != nil {
		return cfg.skipper(c)
	}
	if shouldSkipMethod(c, cfg) {
		return true
	}
	if shouldSkipIP(c, cfg) {
		return true
	}
	return false
}

func defaultMiddlewareConfig() *middlewareConfig {
	return &middlewareConfig{
		sensitiveFieldKeywords: appendSensitiveValues(nil, defaultSensitiveFieldKeywords...),
		sensitiveHeaders:       appendSensitiveValues(nil, defaultSensitiveHeaders...),
	}
}

func shouldSkipMethod(c *gin.Context, cfg *middlewareConfig) bool {
	if c == nil || c.Request == nil || cfg == nil || len(cfg.skipMethods) == 0 {
		return false
	}
	method := strings.ToUpper(strings.TrimSpace(c.Request.Method))
	_, ok := cfg.skipMethods[method]
	return ok
}

func shouldSkipIP(c *gin.Context, cfg *middlewareConfig) bool {
	if c == nil || cfg == nil || (len(cfg.skipIPAddrs) == 0 && len(cfg.skipIPPrefixes) == 0) {
		return false
	}
	addr, err := netip.ParseAddr(c.ClientIP())
	if err != nil {
		return false
	}
	if _, ok := cfg.skipIPAddrs[addr]; ok {
		return true
	}
	for _, prefix := range cfg.skipIPPrefixes {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

func normalizeSkipMethods(methods ...string) map[string]struct{} {
	out := make(map[string]struct{}, len(methods))
	for _, method := range methods {
		method = strings.ToUpper(strings.TrimSpace(method))
		if method == "" {
			continue
		}
		out[method] = struct{}{}
	}
	return out
}

func normalizeSkipIPs(values ...string) (map[netip.Addr]struct{}, []netip.Prefix) {
	addrs := make(map[netip.Addr]struct{}, len(values))
	prefixes := make([]netip.Prefix, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if strings.Contains(value, "/") {
			prefix, err := netip.ParsePrefix(value)
			if err == nil {
				prefixes = append(prefixes, prefix)
			}
			continue
		}
		addr, err := netip.ParseAddr(value)
		if err == nil {
			addrs[addr] = struct{}{}
		}
	}
	return addrs, prefixes
}

// maxReadBody is the upper bound for reading request bodies into memory.
const maxReadBody = 1 << 20 // 1MB

func captureReqBody(c *gin.Context, cfg *middlewareConfig) []byte {
	if c.Request.Body == nil {
		return nil
	}
	contentType := c.Request.Header.Get("Content-Type")
	mediaType, _, _ := mime.ParseMediaType(contentType)

	if mediaType == "multipart/form-data" {
		fields, err := GetRequestBody(c)
		if err != nil {
			return nil
		}
		if len(fields) > 0 {
			redactFields(fields, cfg.sensitiveFieldKeywords)
			b, err := jsonx.Marshal(fields)
			if err != nil {
				return nil
			}
			return b
		}
		return nil
	}

	if isBinContentType(contentType) {
		return []byte("(binary body omitted)")
	}

	bodyBytes, truncated, err := readBodySample(c.Request)
	if err != nil {
		return nil
	}
	if len(bodyBytes) == 0 {
		return nil
	}

	if mediaType == "application/json" {
		if truncated {
			return []byte("(json body omitted: too large)")
		}
		if summarized := summarizeJSON(bodyBytes, cfg.sensitiveFieldKeywords); len(summarized) > 0 {
			return summarized
		}
	}

	if mediaType == "application/x-www-form-urlencoded" {
		if truncated {
			return []byte("(form body omitted: too large)")
		}
		if summarized := summarizeForm(bodyBytes, cfg.sensitiveFieldKeywords); len(summarized) > 0 {
			return summarized
		}
	}

	return truncateBody(bodyBytes, truncated)
}

func readBodySample(req *http.Request) ([]byte, bool, error) {
	original := req.Body
	bodyBytes, err := io.ReadAll(io.LimitReader(original, maxReadBody+1))
	req.Body = replayBody{
		Reader: io.MultiReader(bytes.NewReader(bodyBytes), original),
		closer: original,
	}
	if err != nil {
		return nil, false, err
	}

	if len(bodyBytes) > maxReadBody {
		return bodyBytes[:maxReadBody], true, nil
	}
	return bodyBytes, false, nil
}

func truncateBody(body []byte, truncated bool) []byte {
	if len(body) > maxBodySize {
		out := make([]byte, 0, maxBodySize+14)
		out = append(out, body[:maxBodySize]...)
		out = append(out, []byte("...(truncated)")...)
		return out
	}
	out := make([]byte, len(body), len(body)+14)
	copy(out, body)
	if truncated {
		out = append(out, []byte("...(truncated)")...)
	}
	return out
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

var defaultSensitiveFieldKeywords = []string{
	"authorization",
	"cookie",
	"password",
	"token",
	"secret",
}

// summarizeJSON replaces large string fields with a short placeholder.
func summarizeJSON(body []byte, sensitiveFieldKeywords []string) []byte {
	var data map[string]interface{}
	if err := jsonx.Unmarshal(body, &data); err != nil {
		return nil
	}
	walkAndSummarize(data, sensitiveFieldKeywords)
	b, err := jsonx.Marshal(data)
	if err != nil {
		return nil
	}
	return b
}

func walkAndSummarize(data map[string]interface{}, sensitiveFieldKeywords []string) {
	for k, v := range data {
		if isSensitiveField(k, sensitiveFieldKeywords) {
			data[k] = "(redacted)"
			continue
		}
		switch val := v.(type) {
		case string:
			if len(val) > maxFieldLen {
				data[k] = fmt.Sprintf("(string: %d chars)", len(val))
			}
		case map[string]interface{}:
			walkAndSummarize(val, sensitiveFieldKeywords)
		case []interface{}:
			walkAndSummarizeSlice(val, sensitiveFieldKeywords)
		}
	}
}

func summarizeForm(body []byte, sensitiveFieldKeywords []string) []byte {
	values, err := url.ParseQuery(string(body))
	if err != nil {
		return nil
	}
	data := make(map[string]interface{}, len(values))
	for k, v := range values {
		if isSensitiveField(k, sensitiveFieldKeywords) {
			data[k] = "(redacted)"
			continue
		}
		if len(v) == 1 {
			data[k] = v[0]
		} else {
			data[k] = v
		}
	}
	b, err := jsonx.Marshal(data)
	if err != nil {
		return nil
	}
	return b
}

func redactFields(data map[string]interface{}, sensitiveFieldKeywords []string) {
	for k, v := range data {
		if isSensitiveField(k, sensitiveFieldKeywords) {
			data[k] = "(redacted)"
			continue
		}
		switch val := v.(type) {
		case map[string]interface{}:
			redactFields(val, sensitiveFieldKeywords)
		case []interface{}:
			redactFieldSlice(val, sensitiveFieldKeywords)
		}
	}
}

func redactFieldSlice(values []interface{}, sensitiveFieldKeywords []string) {
	for _, v := range values {
		switch val := v.(type) {
		case map[string]interface{}:
			redactFields(val, sensitiveFieldKeywords)
		case []interface{}:
			redactFieldSlice(val, sensitiveFieldKeywords)
		}
	}
}

func walkAndSummarizeSlice(values []interface{}, sensitiveFieldKeywords []string) {
	for _, v := range values {
		switch val := v.(type) {
		case map[string]interface{}:
			walkAndSummarize(val, sensitiveFieldKeywords)
		case []interface{}:
			walkAndSummarizeSlice(val, sensitiveFieldKeywords)
		}
	}
}

func isSensitiveField(key string, sensitiveFieldKeywords []string) bool {
	key = strings.ToLower(key)
	for _, keyword := range sensitiveFieldKeywords {
		keyword = strings.ToLower(strings.TrimSpace(keyword))
		if keyword != "" && strings.Contains(key, keyword) {
			return true
		}
	}
	return false
}

func appendSensitiveValues(base []string, values ...string) []string {
	out := make([]string, 0, len(base)+len(values))
	seen := make(map[string]struct{}, len(base)+len(values))
	for _, value := range base {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func normalizeSensitiveValues(values ...string) []string {
	return appendSensitiveValues(nil, values...)
}
