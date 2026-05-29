package risk

import (
	"context"
	"time"
)

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
