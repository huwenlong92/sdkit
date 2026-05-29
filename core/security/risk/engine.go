package risk

import (
	"context"
	"errors"
	"time"
)

var ErrMissingStore = errors.New("risk: missing store")

type Engine struct {
	store   Store
	counter Counter
	logger  *Logger
	now     func() time.Time
}

type Option func(*Engine)

func WithNow(now func() time.Time) Option {
	return func(e *Engine) {
		if now != nil {
			e.now = now
		}
	}
}

func WithCounter(counter Counter) Option {
	return func(e *Engine) {
		e.counter = counter
	}
}

func WithLogger(logger *Logger) Option {
	return func(e *Engine) {
		e.logger = logger
	}
}

func NewEngine(store Store, opts ...Option) *Engine {
	e := &Engine{
		store: store,
		now:   time.Now,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(e)
		}
	}
	return e
}

func (e *Engine) Evaluate(ctx context.Context, event Event) (*Decision, error) {
	if e == nil || e.store == nil {
		return nil, ErrMissingStore
	}

	event = normalizeEvent(event)
	now := e.now()
	decision := &Decision{
		Passed:    true,
		Service:   event.Service,
		Scene:     event.Scene,
		Event:     event.Event,
		Level:     firstNonEmpty(event.Level, LevelLow),
		Score:     event.Score,
		Action:    firstNonEmpty(event.Action, ActionAllow),
		Status:    StatusAllow,
		Reasons:   make([]map[string]any, 0),
		Hits:      make([]HitDecision, 0),
		CreatedAt: now,
	}

	scene, err := e.store.LoadScene(ctx, event.Service, event.Scene)
	if err != nil {
		return nil, err
	}
	applySceneDefaults(decision, scene)

	if hit, err := e.store.MatchList(ctx, event, "white"); err != nil {
		return nil, err
	} else if hit != nil {
		applyHit(decision, HitDecision{
			RuleID:          hit.ID,
			RuleCode:        "white_list",
			RuleName:        "白名单",
			RuleType:        RuleTypeList,
			Level:           LevelLow,
			Score:           0,
			Action:          ActionAllow,
			ResponseCode:    hit.ResponseCode,
			ResponseMessage: hit.ResponseMessage,
			ResponseAction:  hit.ResponseAction,
			ResponsePayload: hit.ResponsePayload,
			HTTPStatus:      hit.HTTPStatus,
			Reason: map[string]any{
				"list_type":    hit.ListType,
				"target_type":  hit.TargetType,
				"target_value": hit.TargetValue,
				"scene":        hit.Scene,
				"reason":       hit.Reason,
			},
		})
		return decision, e.saveDecision(ctx, event, decision)
	}

	if hit, err := e.store.MatchList(ctx, event, "black"); err != nil {
		return nil, err
	} else if hit != nil {
		applyHit(decision, HitDecision{
			RuleID:          hit.ID,
			RuleCode:        "black_list",
			RuleName:        "黑名单",
			RuleType:        RuleTypeList,
			Level:           LevelHigh,
			Score:           100,
			Action:          ActionDeny,
			ResponseCode:    hit.ResponseCode,
			ResponseMessage: hit.ResponseMessage,
			ResponseAction:  hit.ResponseAction,
			ResponsePayload: hit.ResponsePayload,
			HTTPStatus:      hit.HTTPStatus,
			Reason: map[string]any{
				"list_type":    hit.ListType,
				"target_type":  hit.TargetType,
				"target_value": hit.TargetValue,
				"scene":        hit.Scene,
				"reason":       hit.Reason,
			},
		})
	}

	if err := e.evaluateFrequencyRules(ctx, event, now, decision); err != nil {
		return nil, err
	}
	return decision, e.saveDecision(ctx, event, decision)
}

func (e *Engine) saveDecision(ctx context.Context, event Event, decision *Decision) error {
	if e.logger != nil && e.logger.Push(event, decision) {
		return nil
	}
	return e.store.SaveDecision(ctx, event, decision)
}
