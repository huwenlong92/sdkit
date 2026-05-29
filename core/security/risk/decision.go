package risk

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
