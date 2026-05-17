package realtime

import (
	"context"
	"encoding/json"
	"fmt"
)

type ActionMessage struct {
	Action    string
	RequestID string
	Headers   map[string]string
	Payload   []byte
}

type ActionContext struct {
	ctx context.Context

	Client   *Client
	Identity *Identity
	Event    *Event
	Gateway  Gateway
	Values   map[string]any
	aborted  bool

	Message ActionMessage
	Raw     []byte

	handlers []HandlerFunc
	index    int
}

func (a *ActionContext) Context() context.Context {
	if a == nil || a.ctx == nil {
		return context.Background()
	}
	return a.ctx
}

func (a *ActionContext) SetContext(ctx context.Context) {
	if a == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	a.ctx = ctx
}

func (a *ActionContext) Next() error {
	if a == nil {
		return ErrInvalidAction
	}
	if a.aborted {
		return nil
	}
	a.index++
	if a.index >= len(a.handlers) {
		return nil
	}
	handler := a.handlers[a.index]
	if handler == nil {
		return ErrNilActionHandler
	}
	return handler(a)
}

func (a *ActionContext) SetHandlers(handlers ...HandlerFunc) {
	if a == nil {
		return
	}
	a.handlers = append([]HandlerFunc(nil), handlers...)
	a.index = -1
	a.aborted = false
}

func (a *ActionContext) RunHandlers(handlers ...HandlerFunc) error {
	if a == nil {
		return ErrInvalidAction
	}
	a.handlers = handlers
	a.index = -1
	a.aborted = false
	return a.Next()
}

func (a *ActionContext) Bind(v any) error {
	if a == nil || a.Event == nil {
		return ErrInvalidAction
	}
	switch data := a.Event.Data.(type) {
	case nil:
		return json.Unmarshal([]byte("{}"), v)
	case []byte:
		if len(data) == 0 {
			data = []byte("{}")
		}
		return json.Unmarshal(data, v)
	case json.RawMessage:
		if len(data) == 0 {
			data = json.RawMessage(`{}`)
		}
		return json.Unmarshal(data, v)
	default:
		raw, err := json.Marshal(data)
		if err != nil {
			return err
		}
		return json.Unmarshal(raw, v)
	}
}

func (a *ActionContext) Reply(action string, data any) error {
	if a == nil {
		return ErrInvalidAction
	}
	if a.Gateway == nil {
		return ErrNilGateway
	}
	return a.Gateway.PushClient(a.contextOrBackground(), a.ClientID(), &Event{
		Action:    action,
		RequestID: a.requestID(),
		TraceID:   a.traceID(),
		Headers:   a.headers(),
		Data:      data,
	})
}

func (a *ActionContext) PushUser(userID string, evt *Event) error {
	if a == nil {
		return ErrInvalidAction
	}
	if a.Gateway == nil {
		return ErrNilGateway
	}
	return a.Gateway.PushUser(a.contextOrBackground(), userID, evt)
}

func (a *ActionContext) PushRoom(roomID string, evt *Event) error {
	if a == nil {
		return ErrInvalidAction
	}
	if a.Gateway == nil {
		return ErrNilGateway
	}
	return a.Gateway.PushRoom(a.contextOrBackground(), roomID, evt)
}

func (a *ActionContext) UserID() string {
	if a == nil {
		return ""
	}
	if a.Identity != nil {
		return a.Identity.Normalize().ID
	}
	if a.Client != nil && a.Client.Identity != nil {
		return a.Client.Identity.Normalize().ID
	}
	return ""
}

func (a *ActionContext) ClientID() string {
	if a == nil || a.Client == nil {
		return ""
	}
	return a.Client.ID
}

func (a *ActionContext) Abort() {
	if a == nil {
		return
	}
	a.aborted = true
	a.index = len(a.handlers)
}

func (a *ActionContext) IsAborted() bool {
	if a == nil {
		return false
	}
	return a.aborted
}

func (a *ActionContext) Set(key string, value any) {
	if a == nil || key == "" {
		return
	}
	if a.Values == nil {
		a.Values = make(map[string]any)
	}
	a.Values[key] = value
}

func (a *ActionContext) Get(key string) any {
	if a == nil || a.Values == nil {
		return nil
	}
	return a.Values[key]
}

func (a *ActionContext) contextOrBackground() context.Context {
	if a == nil || a.ctx == nil {
		return context.Background()
	}
	return a.ctx
}

func (a *ActionContext) requestID() string {
	if a == nil {
		return ""
	}
	if a.Event != nil && a.Event.RequestID != "" {
		return a.Event.RequestID
	}
	return a.Message.RequestID
}

func (a *ActionContext) traceID() string {
	if a == nil || a.Event == nil {
		return ""
	}
	return a.Event.TraceID
}

func (a *ActionContext) headers() map[string]string {
	if a == nil || a.Event == nil {
		return nil
	}
	return a.Event.Headers
}

type HandlerFunc func(action *ActionContext) error
type ActionHandlerFunc = HandlerFunc

type MiddlewareFunc func(HandlerFunc) HandlerFunc

type Route struct {
	Action     string
	Middleware []MiddlewareFunc
	Handler    HandlerFunc
	Compiled   HandlerFunc
	Handlers   []HandlerFunc
}

type Router interface {
	Use(middleware ...MiddlewareFunc)
	On(action string, handlers ...HandlerFunc)
	Group(prefix string, middleware ...MiddlewareFunc) Router
	Match(action string) (*Route, bool)
}

type ActionCodec interface {
	DecodeAction(payload []byte) (ActionMessage, error)
	EncodeAction(message ActionMessage) ([]byte, error)
}

type ActionError struct {
	Code    string
	Message string
	Err     error
}

func NewActionError(code string, message string, err error) *ActionError {
	return &ActionError{Code: code, Message: message, Err: err}
}

func (e *ActionError) Error() string {
	if e == nil {
		return ""
	}
	switch {
	case e.Message != "" && e.Code != "":
		return fmt.Sprintf("%s: %s", e.Code, e.Message)
	case e.Message != "":
		return e.Message
	case e.Code != "":
		return e.Code
	case e.Err != nil:
		return e.Err.Error()
	default:
		return "realtime action error"
	}
}

func (e *ActionError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}
