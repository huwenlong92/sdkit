package risk

import (
	"context"
	"testing"
)

type staticChecker struct{}

func (staticChecker) Name() string { return "static" }

func (staticChecker) Check(ctx context.Context, rc *Context) (*CheckResult, error) {
	return &CheckResult{Passed: false, NeedVerify: true, Score: 40, Actions: []Action{ActionVerify}}, ctx.Err()
}

func TestManagerAggregatesResult(t *testing.T) {
	got, err := NewManager(nil, staticChecker{}).Check(context.Background(), &Context{Scene: "x"})
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if got.Passed || !got.NeedVerify || got.Score != 40 {
		t.Fatalf("bad result: %+v", got)
	}
}
