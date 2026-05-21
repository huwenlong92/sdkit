package checkers

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/huwenlong92/sdkit/core/security/audit"
	"github.com/huwenlong92/sdkit/core/security/risk"
	"github.com/huwenlong92/sdkit/core/security/state"
	securitycrypto "github.com/huwenlong92/sdkit/pkg/security/crypto"
)

func TestLoginFailScenario(t *testing.T) {
	ctx := context.Background()
	store := state.NewMemoryStore()
	writer := audit.NewMemoryWriter()
	manager := risk.NewManager(writer, NewLoginFailChecker(store))
	rc := &risk.Context{Scene: risk.SceneLogin, UID: 7, IP: "1.1.1.1", Extra: map[string]any{"event": "login_failed"}}
	for i := 0; i < 4; i++ {
		got, err := manager.Check(ctx, rc)
		if err != nil {
			t.Fatalf("check: %v", err)
		}
		if got.NeedCaptcha || got.Blocked {
			t.Fatalf("attempt %d should pass soft checks: %+v", i+1, got)
		}
	}
	got, err := manager.Check(ctx, rc)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if !got.NeedCaptcha {
		t.Fatalf("5th failure should need captcha: %+v", got)
	}
	for i := 0; i < 5; i++ {
		got, err = manager.Check(ctx, rc)
		if err != nil {
			t.Fatalf("check: %v", err)
		}
	}
	if !got.Blocked {
		t.Fatalf("10th failure should block: %+v", got)
	}
	if ttl, ok, err := store.TTL(ctx, "security:block:uid:7"); err != nil || !ok || ttl <= 0 {
		t.Fatalf("block ttl missing ttl=%s ok=%v err=%v", ttl, ok, err)
	}
	if !hasEvent(writer.Events(), "login_uid_blocked") {
		t.Fatalf("missing block event: %+v", writer.Events())
	}
}

func TestSMSScenario(t *testing.T) {
	ctx := context.Background()
	store := state.NewMemoryStore()
	writer := audit.NewMemoryWriter()
	checker := NewSMSChecker(store)
	checker.CodeTTL = 20 * time.Millisecond
	manager := risk.NewManager(writer, checker)
	rc := &risk.Context{Scene: risk.SceneSMS, IP: "2.2.2.2", Phone: "13800000000", Extra: map[string]any{"event": "sms_send", "code": "123456"}}
	got, err := manager.Check(ctx, rc)
	if err != nil || !got.Passed {
		t.Fatalf("first send got=%+v err=%v", got, err)
	}
	got, err = manager.Check(ctx, rc)
	if err != nil {
		t.Fatalf("cooldown check: %v", err)
	}
	if got.Passed {
		t.Fatalf("cooldown should reject")
	}
	for i := 0; i < 21; i++ {
		_ = store.Delete(ctx, "security:sms:cooldown:13900000000")
		got, err = manager.Check(ctx, &risk.Context{Scene: risk.SceneSMS, IP: "3.3.3.3", Phone: "13900000000", Extra: map[string]any{"event": "sms_send", "code": "111111"}})
		if err != nil {
			t.Fatalf("daily check: %v", err)
		}
	}
	if !got.Blocked {
		t.Fatalf("daily limit should block: %+v", got)
	}
	time.Sleep(30 * time.Millisecond)
	got, err = manager.Check(ctx, &risk.Context{Scene: risk.SceneSMS, Phone: "13800000000", Extra: map[string]any{"event": "sms_verify", "code": "123456"}})
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if got.Passed {
		t.Fatalf("expired code should not pass")
	}
	if !hasEvent(writer.Events(), "sms_send") || !hasEvent(writer.Events(), "sms_limited") {
		t.Fatalf("missing sms events: %+v", writer.Events())
	}
}

func TestSignatureScenario(t *testing.T) {
	ctx := context.Background()
	store := state.NewMemoryStore()
	writer := audit.NewMemoryWriter()
	secret := []byte("secret")
	checker := NewSignatureChecker(store, secret)
	checker.IPBlockAfter = 3
	manager := risk.NewManager(writer, checker)
	now := strconv.FormatInt(time.Now().Unix(), 10)
	body := []byte(`{"a":1}`)
	rc := signedRC(secret, now, "nonce-1", body)
	got, err := manager.Check(ctx, rc)
	if err != nil || !got.Passed {
		t.Fatalf("valid signature got=%+v err=%v", got, err)
	}
	got, err = manager.Check(ctx, rc)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if got.Passed {
		t.Fatalf("replay should fail")
	}
	bad := signedRC(secret, now, "nonce-2", body)
	bad.Headers["U-Signature"] = "00"
	got, err = manager.Check(ctx, bad)
	if err != nil {
		t.Fatalf("bad signature: %v", err)
	}
	if got.Passed {
		t.Fatalf("bad signature should fail")
	}
	expired := signedRC(secret, strconv.FormatInt(time.Now().Add(-10*time.Minute).Unix(), 10), "nonce-3", body)
	got, err = manager.Check(ctx, expired)
	if err != nil {
		t.Fatalf("expired: %v", err)
	}
	if got.Passed {
		t.Fatalf("expired timestamp should fail")
	}
	for i := 0; i < 2; i++ {
		b := signedRC(secret, now, "nonce-bad-"+strconv.Itoa(i), body)
		b.Headers["U-Signature"] = "00"
		got, err = manager.Check(ctx, b)
		if err != nil {
			t.Fatalf("block threshold: %v", err)
		}
	}
	if !got.Blocked {
		t.Fatalf("signature failures should block ip: %+v", got)
	}
	if !hasEvent(writer.Events(), "signature_invalid") || !hasEvent(writer.Events(), "nonce_replay") {
		t.Fatalf("missing signature events: %+v", writer.Events())
	}
}

func TestAdminScenario(t *testing.T) {
	ctx := context.Background()
	store := state.NewMemoryStore()
	writer := audit.NewMemoryWriter()
	manager := risk.NewManager(writer, NewAdminChecker(store))
	rc := &risk.Context{Scene: risk.SceneAdmin, IP: "4.4.4.4", Path: "/admin/login", Extra: map[string]any{"event": "admin_login_failed"}}
	var got *risk.Result
	var err error
	for i := 0; i < 5; i++ {
		got, err = manager.Check(ctx, rc)
		if err != nil {
			t.Fatalf("admin: %v", err)
		}
	}
	if !got.NeedCaptcha {
		t.Fatalf("admin 5th failure should need captcha")
	}
	for i := 0; i < 5; i++ {
		got, err = manager.Check(ctx, rc)
		if err != nil {
			t.Fatalf("admin block: %v", err)
		}
	}
	if !got.Blocked {
		t.Fatalf("admin 10th failure should block")
	}
	got, err = manager.Check(ctx, &risk.Context{Scene: risk.SceneAdmin, IP: "5.5.5.5", Path: "/.env"})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if got.Passed || got.Score == 0 {
		t.Fatalf("scan should add risk: %+v", got)
	}
	if !hasEvent(writer.Events(), "admin_scan_detected") {
		t.Fatalf("missing scan event")
	}
}

func TestCommentScenario(t *testing.T) {
	ctx := context.Background()
	store := state.NewMemoryStore()
	writer := audit.NewMemoryWriter()
	manager := risk.NewManager(writer, NewCommentChecker(store, "blocked-word"))
	got, err := manager.Check(ctx, &risk.Context{Scene: risk.SceneComment, UID: 1, IP: "6.6.6.6", DeviceID: "d1", Extra: map[string]any{"content": "hello"}})
	if err != nil || !got.Passed {
		t.Fatalf("normal comment got=%+v err=%v", got, err)
	}
	for i := 0; i < 10; i++ {
		got, err = manager.Check(ctx, &risk.Context{Scene: risk.SceneComment, UID: 1, IP: "6.6.6.7", DeviceID: "d2", Extra: map[string]any{"content": "hello"}})
		if err != nil {
			t.Fatalf("comment limit: %v", err)
		}
	}
	if !got.NeedCaptcha {
		t.Fatalf("uid limit should need captcha/review: %+v", got)
	}
	got, err = manager.Check(ctx, &risk.Context{Scene: risk.SceneComment, UID: 2, IP: "6.6.6.8", Extra: map[string]any{"content": "has blocked-word"}})
	if err != nil {
		t.Fatalf("sensitive: %v", err)
	}
	if got.Passed {
		t.Fatalf("sensitive content should review")
	}
	if !hasEvent(writer.Events(), "sensitive_content_hit") {
		t.Fatalf("missing sensitive event")
	}
}

func TestRegisterScenario(t *testing.T) {
	ctx := context.Background()
	store := state.NewMemoryStore()
	writer := audit.NewMemoryWriter()
	manager := risk.NewManager(writer, NewRegisterChecker(store))
	var got *risk.Result
	var err error
	got, err = manager.Check(ctx, &risk.Context{Scene: risk.SceneRegister, IP: "7.7.7.7", DeviceID: "dev-normal"})
	if err != nil || !got.Passed {
		t.Fatalf("normal register got=%+v err=%v", got, err)
	}
	for i := 0; i < 10; i++ {
		got, err = manager.Check(ctx, &risk.Context{Scene: risk.SceneRegister, IP: "7.7.7.8"})
		if err != nil {
			t.Fatalf("register ip: %v", err)
		}
	}
	if !got.NeedCaptcha {
		t.Fatalf("ip threshold should need captcha")
	}
	for i := 0; i < 5; i++ {
		got, err = manager.Check(ctx, &risk.Context{Scene: risk.SceneRegister, DeviceID: "dev-risk"})
		if err != nil {
			t.Fatalf("register device: %v", err)
		}
	}
	if !got.NeedCaptcha {
		t.Fatalf("device threshold should need captcha")
	}
	for i := 0; i < 5; i++ {
		got, err = manager.Check(ctx, &risk.Context{Scene: risk.SceneRegister, DeviceID: "dev-risk"})
		if err != nil {
			t.Fatalf("register device block: %v", err)
		}
	}
	if !got.Blocked {
		t.Fatalf("device threshold should block")
	}
	if !hasEvent(writer.Events(), "register_device_blocked") {
		t.Fatalf("missing register event")
	}
}

func TestAnomalyLoginScenario(t *testing.T) {
	ctx := context.Background()
	store := state.NewMemoryStore()
	writer := audit.NewMemoryWriter()
	checker := NewAnomalyLoginChecker(store)
	manager := risk.NewManager(writer, checker)
	if err := store.Set(ctx, "security:device_seen:9:dev-old", "1", time.Hour); err != nil {
		t.Fatalf("set device: %v", err)
	}
	if err := store.Set(ctx, "security:last_login:uid:9", "CN|ua-old", time.Hour); err != nil {
		t.Fatalf("set login: %v", err)
	}
	got, err := manager.Check(ctx, &risk.Context{Scene: risk.SceneLoginRisk, UID: 9, DeviceID: "dev-old", Region: "CN", UA: "ua-old"})
	if err != nil || !got.Passed || got.Score != 0 {
		t.Fatalf("old device got=%+v err=%v", got, err)
	}
	got, err = manager.Check(ctx, &risk.Context{Scene: risk.SceneLoginRisk, UID: 9, DeviceID: "dev-new", Region: "US", UA: "ua-new"})
	if err != nil {
		t.Fatalf("anomaly: %v", err)
	}
	if !got.NeedVerify || got.Score < 40 {
		t.Fatalf("anomaly should need verify: %+v", got)
	}
	if !hasEvent(writer.Events(), "abnormal_login") {
		t.Fatalf("missing abnormal login event")
	}
}

func signedRC(secret []byte, ts, nonce string, body []byte) *risk.Context {
	sig := securitycrypto.SignHMACSHA256(secret, "POST", "/openapi/order/create", ts, nonce, body)
	return &risk.Context{
		Scene:  risk.SceneOpenAPI,
		IP:     "8.8.8.8",
		Method: "POST",
		Path:   "/openapi/order/create",
		Body:   body,
		Headers: map[string]string{
			"U-Timestamp": ts,
			"U-Nonce":     nonce,
			"U-Signature": sig,
		},
	}
}

func hasEvent(events []*audit.Event, name string) bool {
	for _, event := range events {
		if event.Event == name {
			return true
		}
	}
	return false
}
