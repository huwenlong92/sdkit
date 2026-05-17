package realtime

import "encoding/json"

type Context = ActionContext

func (a *ActionContext) ShouldBindJSON(dst any) error {
	if a == nil {
		return ErrInvalidAction
	}
	payload := a.Message.Payload
	if len(payload) == 0 {
		payload = []byte("{}")
	}
	return json.Unmarshal(payload, dst)
}

func (a *ActionContext) Control(event string, data any) error {
	if a == nil {
		return ErrInvalidAction
	}
	return a.Reply(event, data)
}

func (a *ActionContext) ActionError(code string, message string, err error) error {
	return NewActionError(code, message, err)
}
