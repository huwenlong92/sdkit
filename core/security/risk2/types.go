package risk2

import "time"

const (
	ActionAllow   = "allow"
	ActionReview  = "review"
	ActionCaptcha = "captcha"
	ActionLimit   = "limit"
	ActionDeny    = "deny"
	ActionBlock   = "block"

	LevelLow      = "low"
	LevelMedium   = "medium"
	LevelHigh     = "high"
	LevelCritical = "critical"

	StatusAllow   = "allow"
	StatusHit     = "hit"
	StatusBlocked = "blocked"

	RuleTypeList      = "list"
	RuleTypeFrequency = "frequency"

	ResponseActionNone       = "none"
	ResponseActionToast      = "toast"
	ResponseActionCaptcha    = "captcha"
	ResponseActionAppeal     = "appeal"
	ResponseActionRetryLater = "retry_later"
	ResponseActionContact    = "contact"
)

type Event struct {
	Service     string         `json:"service"`
	Scene       string         `json:"scene"`
	Event       string         `json:"event"`
	Level       string         `json:"level"`
	SubjectType string         `json:"subject_type"`
	SubjectID   string         `json:"subject_id"`
	UID         string         `json:"uid"`
	IP          string         `json:"ip"`
	DeviceID    string         `json:"device_id"`
	TrackID     string         `json:"track_id"`
	TraceID     string         `json:"trace_id"`
	RequestID   string         `json:"request_id"`
	Score       int            `json:"score"`
	Action      string         `json:"action"`
	Extra       map[string]any `json:"extra"`
}

type Scene struct {
	Service        string `json:"service"`
	Code           string `json:"code"`
	DefaultLevel   string `json:"default_level"`
	DefaultAction  string `json:"default_action"`
	ScoreThreshold int    `json:"score_threshold"`
}

type ListRule struct {
	ID              int64          `json:"id"`
	ListType        string         `json:"list_type"`
	TargetType      string         `json:"target_type"`
	TargetValue     string         `json:"target_value"`
	Scene           string         `json:"scene"`
	Reason          string         `json:"reason"`
	ResponseCode    int            `json:"response_code"`
	ResponseMessage string         `json:"response_message"`
	ResponseAction  string         `json:"response_action"`
	ResponsePayload map[string]any `json:"response_payload"`
	HTTPStatus      int            `json:"http_status"`
}

type FrequencyRule struct {
	ID              int64          `json:"id"`
	Code            string         `json:"code"`
	Name            string         `json:"name"`
	Event           string         `json:"event"`
	TargetType      string         `json:"target_type"`
	WindowSeconds   int            `json:"window_seconds"`
	LimitCount      int            `json:"limit_count"`
	Level           string         `json:"level"`
	Action          string         `json:"action"`
	Score           int            `json:"score"`
	ResponseCode    int            `json:"response_code"`
	ResponseMessage string         `json:"response_message"`
	ResponseAction  string         `json:"response_action"`
	ResponsePayload map[string]any `json:"response_payload"`
	HTTPStatus      int            `json:"http_status"`
}

type EventCountQuery struct {
	Service     string
	Scene       string
	Event       string
	TargetType  string
	TargetValue string
	Since       time.Time
}

type CounterKey struct {
	Service       string
	Scene         string
	Event         string
	TargetType    string
	TargetValue   string
	WindowSeconds int
}

type Decision struct {
	Passed          bool             `json:"passed"`
	EventID         int64            `json:"event_id"`
	Service         string           `json:"service"`
	Scene           string           `json:"scene"`
	Event           string           `json:"event"`
	Level           string           `json:"level"`
	Score           int              `json:"score"`
	Action          string           `json:"action"`
	Status          string           `json:"status"`
	ResponseCode    int              `json:"response_code"`
	ResponseMessage string           `json:"response_message"`
	ResponseAction  string           `json:"response_action"`
	ResponsePayload map[string]any   `json:"response_payload"`
	HTTPStatus      int              `json:"http_status"`
	Reasons         []map[string]any `json:"reasons"`
	Hits            []HitDecision    `json:"hits"`
	CreatedAt       time.Time        `json:"created_at"`
}

type HitDecision struct {
	ID              int64          `json:"id"`
	RuleID          int64          `json:"rule_id"`
	RuleCode        string         `json:"rule_code"`
	RuleName        string         `json:"rule_name"`
	RuleType        string         `json:"rule_type"`
	Level           string         `json:"level"`
	Score           int            `json:"score"`
	Action          string         `json:"action"`
	ResponseCode    int            `json:"response_code"`
	ResponseMessage string         `json:"response_message"`
	ResponseAction  string         `json:"response_action"`
	ResponsePayload map[string]any `json:"response_payload"`
	HTTPStatus      int            `json:"http_status"`
	Reason          map[string]any `json:"reason"`
	Snapshot        map[string]any `json:"snapshot"`
}
