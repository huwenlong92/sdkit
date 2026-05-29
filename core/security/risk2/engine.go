package risk2

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"
)

var ErrMissingStore = errors.New("risk2: missing store")

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

func (e *Engine) evaluateFrequencyRules(ctx context.Context, event Event, now time.Time, decision *Decision) error {
	rules, err := e.store.ListFrequencyRules(ctx, event)
	if err != nil {
		return err
	}

	for _, rule := range rules {
		targetValue, ok := TargetValue(event, rule.TargetType)
		if !ok {
			continue
		}
		currentCount, err := e.currentFrequencyCount(ctx, event, rule, targetValue, now)
		if err != nil {
			return err
		}
		if currentCount < int64(rule.LimitCount) {
			continue
		}

		applyHit(decision, HitDecision{
			RuleID:          rule.ID,
			RuleCode:        rule.Code,
			RuleName:        rule.Name,
			RuleType:        RuleTypeFrequency,
			Level:           firstNonEmpty(rule.Level, LevelMedium),
			Score:           rule.Score,
			Action:          rule.Action,
			ResponseCode:    rule.ResponseCode,
			ResponseMessage: rule.ResponseMessage,
			ResponseAction:  rule.ResponseAction,
			ResponsePayload: rule.ResponsePayload,
			HTTPStatus:      rule.HTTPStatus,
			Reason: map[string]any{
				"target_type":   rule.TargetType,
				"target_value":  targetValue,
				"window":        rule.WindowSeconds,
				"limit":         rule.LimitCount,
				"current_count": currentCount,
			},
			Snapshot: map[string]any{
				"event":          event.Event,
				"rule_event":     rule.Event,
				"target_type":    rule.TargetType,
				"target_value":   targetValue,
				"window_seconds": rule.WindowSeconds,
				"limit_count":    rule.LimitCount,
				"current_count":  currentCount,
			},
		})
	}
	return nil
}

func (e *Engine) currentFrequencyCount(ctx context.Context, event Event, rule FrequencyRule, targetValue string, now time.Time) (int64, error) {
	if e.counter != nil {
		count, err := e.counter.Incr(ctx, CounterKey{
			Service:       event.Service,
			Scene:         event.Scene,
			Event:         rule.Event,
			TargetType:    rule.TargetType,
			TargetValue:   targetValue,
			WindowSeconds: rule.WindowSeconds,
		})
		if err == nil {
			return count, nil
		}
	}

	count, err := e.store.CountEvents(ctx, EventCountQuery{
		Service:     event.Service,
		Scene:       event.Scene,
		Event:       rule.Event,
		TargetType:  rule.TargetType,
		TargetValue: targetValue,
		Since:       now.Add(-time.Duration(rule.WindowSeconds) * time.Second),
	})
	if err != nil {
		return 0, err
	}
	return count + 1, nil
}

func normalizeEvent(event Event) Event {
	event.Service = firstNonEmpty(strings.TrimSpace(event.Service), "admin")
	event.Scene = strings.TrimSpace(event.Scene)
	event.Event = strings.TrimSpace(event.Event)
	event.Level = strings.TrimSpace(event.Level)
	event.SubjectType = strings.TrimSpace(event.SubjectType)
	event.SubjectID = strings.TrimSpace(event.SubjectID)
	event.UID = strings.TrimSpace(event.UID)
	event.IP = strings.TrimSpace(event.IP)
	event.DeviceID = strings.TrimSpace(event.DeviceID)
	event.TrackID = strings.TrimSpace(event.TrackID)
	event.TraceID = strings.TrimSpace(event.TraceID)
	event.RequestID = strings.TrimSpace(event.RequestID)
	event.Action = strings.TrimSpace(event.Action)
	if event.Extra == nil {
		event.Extra = map[string]any{}
	}
	return event
}

func applySceneDefaults(decision *Decision, scene *Scene) {
	if scene == nil {
		return
	}
	if scene.DefaultLevel != "" && decision.Level == LevelLow {
		decision.Level = scene.DefaultLevel
	}
	if scene.DefaultAction != "" && decision.Action == ActionAllow {
		decision.Action = scene.DefaultAction
	}
	if scene.ScoreThreshold > 0 && decision.Score < scene.ScoreThreshold {
		decision.Score = scene.ScoreThreshold
	}
}

func applyHit(decision *Decision, hit HitDecision) {
	if hit.Reason == nil {
		hit.Reason = map[string]any{}
	}
	if hit.Snapshot == nil {
		hit.Snapshot = map[string]any{}
	}
	if shouldUseHitResponse(decision, hit) {
		decision.ResponseCode = hit.ResponseCode
		decision.ResponseMessage = hit.ResponseMessage
		decision.ResponseAction = hit.ResponseAction
		decision.ResponsePayload = cloneMap(hit.ResponsePayload)
		decision.HTTPStatus = hit.HTTPStatus
	}
	decision.Reasons = append(decision.Reasons, hit.Reason)
	decision.Hits = append(decision.Hits, hit)
	if hit.Score > decision.Score {
		decision.Score = hit.Score
	}
	if levelPriority(hit.Level) > levelPriority(decision.Level) {
		decision.Level = hit.Level
	}
	if actionPriority(hit.Action) > actionPriority(decision.Action) {
		decision.Action = hit.Action
	}
	if actionAllows(decision.Action) {
		decision.Passed = true
		decision.Status = StatusHit
		return
	}
	decision.Passed = false
	decision.Status = StatusBlocked
}

func shouldUseHitResponse(decision *Decision, hit HitDecision) bool {
	currentActionPriority := actionPriority(decision.Action)
	hitActionPriority := actionPriority(hit.Action)
	if hitActionPriority > currentActionPriority {
		return true
	}
	if hitActionPriority < currentActionPriority {
		return false
	}
	return len(decision.Hits) == 0 || hit.Score > decision.Score
}

func TargetValues(event Event) map[string]string {
	targets := make(map[string]string)
	if event.UID != "" {
		targets["uid"] = event.UID
	}
	if event.IP != "" {
		targets["ip"] = event.IP
	}
	if event.DeviceID != "" {
		targets["device_id"] = event.DeviceID
	}
	if event.SubjectID != "" {
		targets["subject_id"] = event.SubjectID
	}
	for _, key := range []string{"account", "mobile", "email"} {
		if value, ok := event.Extra[key].(string); ok && strings.TrimSpace(value) != "" {
			targets[key] = strings.TrimSpace(value)
		}
	}
	return targets
}

func TargetValue(event Event, targetType string) (string, bool) {
	switch targetType {
	case "uid":
		return event.UID, event.UID != ""
	case "ip":
		return event.IP, event.IP != ""
	case "device_id":
		return event.DeviceID, event.DeviceID != ""
	case "subject_id":
		return event.SubjectID, event.SubjectID != ""
	default:
		value, ok := event.Extra[targetType]
		if !ok {
			return "", false
		}
		switch v := value.(type) {
		case string:
			v = strings.TrimSpace(v)
			return v, v != ""
		case int64:
			return strconv.FormatInt(v, 10), true
		case int:
			return strconv.Itoa(v), true
		case float64:
			if v == float64(int64(v)) {
				return strconv.FormatInt(int64(v), 10), true
			}
		}
	}
	return "", false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func actionAllows(action string) bool {
	return action == "" || action == ActionAllow || action == ActionReview
}

func actionPriority(action string) int {
	switch action {
	case ActionBlock:
		return 60
	case ActionDeny:
		return 50
	case ActionLimit:
		return 40
	case ActionCaptcha:
		return 30
	case ActionReview:
		return 20
	case ActionAllow:
		return 10
	default:
		return 0
	}
}

func levelPriority(level string) int {
	switch level {
	case LevelCritical:
		return 40
	case LevelHigh:
		return 30
	case LevelMedium:
		return 20
	case LevelLow:
		return 10
	default:
		return 0
	}
}
