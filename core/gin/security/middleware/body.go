package middleware

import "bytes"

func bytesReader(body []byte) *bytes.Reader {
	return bytes.NewReader(body)
}
