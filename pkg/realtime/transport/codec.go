package transport

import (
	"bytes"
	"encoding/json"

	"github.com/huwenlong92/sdkit/core/realtime"
)

type JSONActionCodec struct{}

func NewJSONActionCodec() JSONActionCodec {
	return JSONActionCodec{}
}

func (JSONActionCodec) DecodeAction(payload []byte) (realtime.ActionMessage, error) {
	var envelope jsonActionEnvelope
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return realtime.ActionMessage{}, err
	}
	messagePayload := envelope.Payload
	if len(messagePayload) == 0 {
		messagePayload = envelope.Data
	}
	return realtime.ActionMessage{
		Action:    envelope.Action,
		RequestID: envelope.RequestID,
		Headers:   envelope.Headers,
		Payload:   messagePayload,
	}, nil
}

func (JSONActionCodec) EncodeAction(message realtime.ActionMessage) ([]byte, error) {
	envelope := jsonActionEnvelope{
		Action:    message.Action,
		RequestID: message.RequestID,
		Headers:   message.Headers,
	}
	if len(message.Payload) > 0 {
		payload := bytes.TrimSpace(message.Payload)
		if !json.Valid(payload) {
			return nil, realtime.ErrInvalidAction
		}
		envelope.Data = payload
	}
	return json.Marshal(envelope)
}

type jsonActionEnvelope struct {
	Action    string            `json:"action"`
	RequestID string            `json:"request_id,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
	Data      json.RawMessage   `json:"data,omitempty"`
	Payload   json.RawMessage   `json:"payload,omitempty"`
}

var _ realtime.ActionCodec = JSONActionCodec{}
