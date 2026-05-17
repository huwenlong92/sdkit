package risk

type Action string

const (
	ActionAllow   Action = "allow"
	ActionCaptcha Action = "captcha"
	ActionVerify  Action = "verify"
	ActionDelay   Action = "delay"
	ActionReview  Action = "review"
	ActionBlock   Action = "block"
)
