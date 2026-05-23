package request

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
)

var (
	ErrNilContext        = errors.New("request: nil context")
	ErrNilHTTPClient     = errors.New("request: nil http client")
	ErrNilTransport      = errors.New("request: nil transport")
	ErrNilBody           = errors.New("request: nil body")
	ErrBodyAlreadySet    = errors.New("request: body already set")
	ErrBodyNotReplayable = errors.New("request: body is not replayable")
	ErrBodyTooLarge      = errors.New("request: response body too large")
	ErrNilMultipartPart  = errors.New("request: multipart part has nil reader")
)

type RequestError struct {
	Method string
	URL    string
	Err    error
}

func (e *RequestError) Error() string {
	if e == nil {
		return ""
	}
	target := e.URL
	if target == "" {
		target = "<unknown>"
	}
	return fmt.Sprintf("request: %s %s failed: %v", e.Method, target, e.Err)
}

func (e *RequestError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

type StatusError struct {
	Response *Response
}

func (e *StatusError) Error() string {
	if e == nil || e.Response == nil {
		return "request: unexpected response status"
	}
	msg := fmt.Sprintf("request: unexpected response status: %s", e.Response.Status)
	if body := strings.TrimSpace(string(e.Response.Body)); body != "" {
		if len(body) > 512 {
			body = body[:512] + "..."
		}
		msg += ": " + body
	}
	return msg
}

func (e *StatusError) StatusCode() int {
	if e == nil || e.Response == nil {
		return 0
	}
	return e.Response.StatusCode
}

func (e *StatusError) Header() http.Header {
	if e == nil || e.Response == nil {
		return nil
	}
	return e.Response.Header.Clone()
}

func (e *StatusError) Body() []byte {
	if e == nil || e.Response == nil {
		return nil
	}
	return append([]byte(nil), e.Response.Body...)
}

type DecodeError struct {
	Err  error
	Body []byte
}

func (e *DecodeError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("request: decode response failed: %v", e.Err)
}

func (e *DecodeError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}
