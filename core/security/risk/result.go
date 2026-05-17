package risk

import "github.com/huwenlong92/sdkit/core/security/audit"

type Reason struct {
	Code    string
	Message string
}

type CheckResult struct {
	Passed      bool
	Score       int
	Actions     []Action
	Reasons     []Reason
	NeedCaptcha bool
	NeedVerify  bool
	Blocked     bool
	Events      []*audit.Event
}

type Result struct {
	Passed      bool
	Score       int
	Actions     []Action
	Reasons     []Reason
	NeedCaptcha bool
	NeedVerify  bool
	Blocked     bool
}
