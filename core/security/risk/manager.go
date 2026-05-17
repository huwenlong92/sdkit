package risk

import (
	"context"

	"github.com/huwenlong92/sdkit/core/security/audit"
)

type Manager struct {
	checkers []Checker
	auditor  audit.Writer
}

func NewManager(auditor audit.Writer, checkers ...Checker) *Manager {
	if auditor == nil {
		auditor = audit.NopWriter{}
	}
	m := &Manager{auditor: auditor}
	for _, checker := range checkers {
		m.Register(checker)
	}
	return m
}

func (m *Manager) Register(checker Checker) {
	if checker != nil {
		m.checkers = append(m.checkers, checker)
	}
}

func (m *Manager) Check(ctx context.Context, rc *Context) (*Result, error) {
	result := &Result{Passed: true}
	for _, checker := range m.checkers {
		cr, err := checker.Check(ctx, rc)
		if err != nil {
			return nil, err
		}
		if cr == nil {
			continue
		}
		result.Score += cr.Score
		result.Actions = append(result.Actions, cr.Actions...)
		result.Reasons = append(result.Reasons, cr.Reasons...)
		result.NeedCaptcha = result.NeedCaptcha || cr.NeedCaptcha
		result.NeedVerify = result.NeedVerify || cr.NeedVerify
		result.Blocked = result.Blocked || cr.Blocked
		if cr.Passed == false || cr.NeedCaptcha || cr.NeedVerify || cr.Blocked {
			result.Passed = false
		}
		if len(cr.Events) > 0 {
			if err := m.auditor.WriteBatch(ctx, cr.Events); err != nil {
				return nil, err
			}
		}
	}
	if len(result.Actions) == 0 {
		result.Actions = []Action{ActionAllow}
	}
	return result, nil
}
