package request

import (
	"bytes"
	"encoding/json"
	"net/http"
)

type Response struct {
	Request    *http.Request
	Raw        *http.Response
	StatusCode int
	Status     string
	Header     http.Header
	Body       []byte
}

func newResponse(req *http.Request, raw *http.Response, body []byte) *Response {
	resp := &Response{
		Request: req,
		Raw:     raw,
		Body:    append([]byte(nil), body...),
	}
	if raw != nil {
		resp.StatusCode = raw.StatusCode
		resp.Status = raw.Status
		resp.Header = raw.Header.Clone()
	}
	return resp
}

func (r *Response) Bytes() []byte {
	if r == nil {
		return nil
	}
	return append([]byte(nil), r.Body...)
}

func (r *Response) String() string {
	if r == nil {
		return ""
	}
	return string(r.Body)
}

func (r *Response) DecodeJSON(v any) error {
	if r == nil {
		return nil
	}
	decoder := json.NewDecoder(bytes.NewReader(r.Body))
	if err := decoder.Decode(v); err != nil {
		return &DecodeError{Err: err, Body: r.Bytes()}
	}
	return nil
}
