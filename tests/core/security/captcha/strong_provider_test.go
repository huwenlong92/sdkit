package captcha_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/huwenlong92/sdkit/core/security/captcha"
	captchastore "github.com/huwenlong92/sdkit/core/security/captcha/store"
)

func TestSliderProviderVerify(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := captchastore.NewMemoryStore()
	provider := captcha.NewSliderProvider(
		store,
		time.Minute,
		captcha.WithSliderSize(180, 90),
		captcha.WithSliderPieceSize(28),
		captcha.WithSliderTolerance(4),
		captcha.WithSliderMinDuration(0),
	)
	challenge, err := provider.Generate(ctx, captcha.GenerateOptions{})
	if err != nil {
		t.Fatalf("generate slider failed: %v", err)
	}
	if challenge.Kind != captcha.KindSlider || challenge.Image == "" {
		t.Fatalf("unexpected challenge: %#v", challenge)
	}
	if challenge.Payload["piece"] == "" {
		t.Fatalf("missing slider piece payload: %#v", challenge.Payload)
	}

	raw, ok, err := store.Get(ctx, challenge.ID)
	if err != nil || !ok {
		t.Fatalf("missing slider state: ok=%v err=%v", ok, err)
	}
	var state struct {
		X int `json:"x"`
		Y int `json:"y"`
	}
	if err := json.Unmarshal([]byte(raw), &state); err != nil {
		t.Fatalf("decode slider state failed: %v", err)
	}
	answer := fmt.Sprintf(`{"x":%d,"y":%d,"duration_ms":300}`, state.X, state.Y)
	if err := provider.Verify(ctx, challenge.ID, answer); err != nil {
		t.Fatalf("verify slider failed: %v", err)
	}
	if _, ok, err := store.Get(ctx, challenge.ID); err != nil || ok {
		t.Fatalf("slider state should be deleted: ok=%v err=%v", ok, err)
	}
}

func TestClickProviderVerify(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := captchastore.NewMemoryStore()
	provider := captcha.NewClickProvider(
		store,
		time.Minute,
		captcha.WithClickSize(180, 90),
		captcha.WithClickTargets(2),
		captcha.WithClickTolerance(5),
	)
	challenge, err := provider.Generate(ctx, captcha.GenerateOptions{})
	if err != nil {
		t.Fatalf("generate click failed: %v", err)
	}
	if challenge.Kind != captcha.KindClick || challenge.Image == "" {
		t.Fatalf("unexpected challenge: %#v", challenge)
	}
	targets, ok := challenge.Payload["targets"].([]string)
	if !ok || len(targets) != 2 {
		t.Fatalf("unexpected click targets: %#v", challenge.Payload)
	}

	raw, ok, err := store.Get(ctx, challenge.ID)
	if err != nil || !ok {
		t.Fatalf("missing click state: ok=%v err=%v", ok, err)
	}
	var state struct {
		Points []struct {
			X int `json:"x"`
			Y int `json:"y"`
		} `json:"points"`
	}
	if err := json.Unmarshal([]byte(raw), &state); err != nil {
		t.Fatalf("decode click state failed: %v", err)
	}
	answer := fmt.Sprintf(`{"points":[{"x":%d,"y":%d},{"x":%d,"y":%d}]}`,
		state.Points[0].X, state.Points[0].Y,
		state.Points[1].X, state.Points[1].Y,
	)
	if err := provider.Verify(ctx, challenge.ID, answer); err != nil {
		t.Fatalf("verify click failed: %v", err)
	}
	if _, ok, err := store.Get(ctx, challenge.ID); err != nil || ok {
		t.Fatalf("click state should be deleted: ok=%v err=%v", ok, err)
	}
}

func TestClientTokenProviderKinds(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	sliderStore := captchastore.NewMemoryStore()
	clickStore := captchastore.NewMemoryStore()
	sliderProvider := captcha.NewClientSliderProvider(sliderStore, time.Minute)
	clickProvider := captcha.NewClientClickProvider(clickStore, time.Minute)

	sliderChallenge, err := sliderProvider.Generate(ctx, captcha.GenerateOptions{})
	if err != nil {
		t.Fatalf("generate client slider failed: %v", err)
	}
	if sliderChallenge.Kind != captcha.KindClientSlider {
		t.Fatalf("slider kind = %s, want %s", sliderChallenge.Kind, captcha.KindClientSlider)
	}
	if err := sliderProvider.Verify(ctx, sliderChallenge.ID, "passed"); err != nil {
		t.Fatalf("verify client slider failed: %v", err)
	}

	clickChallenge, err := clickProvider.Generate(ctx, captcha.GenerateOptions{})
	if err != nil {
		t.Fatalf("generate client click failed: %v", err)
	}
	if clickChallenge.Kind != captcha.KindClientClick {
		t.Fatalf("click kind = %s, want %s", clickChallenge.Kind, captcha.KindClientClick)
	}
	if err := clickProvider.Verify(ctx, clickChallenge.ID, "passed"); err != nil {
		t.Fatalf("verify client click failed: %v", err)
	}
}
