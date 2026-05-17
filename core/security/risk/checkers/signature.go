package checkers

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/huwenlong92/sdkit/core/security/audit"
	securitycrypto "github.com/huwenlong92/sdkit/core/security/crypto"
	"github.com/huwenlong92/sdkit/core/security/risk"
	"github.com/huwenlong92/sdkit/core/security/state"
)

var ErrMissingSecret = errors.New("security signature: missing secret")

type SignatureChecker struct {
	Store        state.Store
	Secret       []byte
	Skew         time.Duration
	NonceTTL     time.Duration
	FailWindow   time.Duration
	BlockTTL     time.Duration
	IPBlockAfter int64
	CheckNonce   bool
}

func NewSignatureChecker(store state.Store, secret []byte) *SignatureChecker {
	return &SignatureChecker{Store: store, Secret: secret, Skew: 5 * time.Minute, NonceTTL: 5 * time.Minute, FailWindow: 5 * time.Minute, BlockTTL: 30 * time.Minute, IPBlockAfter: 10, CheckNonce: true}
}

func (c *SignatureChecker) Name() string { return "signature" }

func (c *SignatureChecker) Check(ctx context.Context, rc *risk.Context) (*risk.CheckResult, error) {
	if rc.Scene != risk.SceneOpenAPI {
		return nil, nil
	}
	if len(c.Secret) == 0 {
		return nil, ErrMissingSecret
	}
	if rc.IP != "" {
		blocked, err := c.Store.Exists(ctx, "security:block:ip:"+rc.IP)
		if err != nil {
			return nil, err
		}
		if blocked {
			return &risk.CheckResult{Passed: false, Blocked: true, Actions: []risk.Action{risk.ActionBlock}, Events: []*audit.Event{event(rc, "openapi_ip_blocked", levelHigh, 100, risk.ActionBlock, "ip blocked")}}, nil
		}
	}
	ts := header(rc, "U-Timestamp")
	nonce := header(rc, "U-Nonce")
	signature := header(rc, "U-Signature")
	if ts == "" || nonce == "" || signature == "" {
		return c.signatureFail(ctx, rc, "signature_invalid", "missing signature header")
	}
	unix, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return c.signatureFail(ctx, rc, "signature_invalid", "invalid timestamp")
	}
	now := time.Now()
	if now.Sub(time.Unix(unix, 0)) > ttlOrDefault(c.Skew, 5*time.Minute) || time.Unix(unix, 0).Sub(now) > ttlOrDefault(c.Skew, 5*time.Minute) {
		return c.signatureFail(ctx, rc, "signature_invalid", "expired timestamp")
	}
	if c.CheckNonce {
		ok, err := c.Store.SetNX(ctx, "security:nonce:"+nonce, "1", ttlOrDefault(c.NonceTTL, 5*time.Minute))
		if err != nil {
			return nil, err
		}
		if !ok {
			return &risk.CheckResult{Passed: false, Actions: []risk.Action{risk.ActionBlock}, Events: []*audit.Event{event(rc, "nonce_replay", levelHigh, 100, risk.ActionBlock, "nonce replay")}}, nil
		}
	}
	expected := securitycrypto.SignHMACSHA256(c.Secret, rc.Method, rc.Path, ts, nonce, rc.Body)
	if !securitycrypto.EqualHMACHex(signature, expected) {
		return c.signatureFail(ctx, rc, "signature_invalid", "invalid signature")
	}
	return &risk.CheckResult{Passed: true, Actions: []risk.Action{risk.ActionAllow}}, nil
}

func (c *SignatureChecker) signatureFail(ctx context.Context, rc *risk.Context, eventName string, reason string) (*risk.CheckResult, error) {
	cr := &risk.CheckResult{Passed: false, Actions: []risk.Action{risk.ActionBlock}, Events: []*audit.Event{event(rc, eventName, levelHigh, 100, risk.ActionBlock, reason)}}
	if rc.IP == "" {
		return cr, nil
	}
	n, err := c.Store.Incr(ctx, "security:signature_fail:ip:"+rc.IP, ttlOrDefault(c.FailWindow, 5*time.Minute))
	if err != nil {
		return nil, err
	}
	if n >= c.IPBlockAfter {
		if err := c.Store.Set(ctx, "security:block:ip:"+rc.IP, "1", ttlOrDefault(c.BlockTTL, 30*time.Minute)); err != nil {
			return nil, err
		}
		cr.Blocked = true
		cr.Events = append(cr.Events, event(rc, "openapi_ip_blocked", levelHigh, 100, risk.ActionBlock, "signature failure threshold reached"))
	}
	return cr, nil
}

func header(rc *risk.Context, key string) string {
	if rc.Headers == nil {
		return ""
	}
	return rc.Headers[key]
}
