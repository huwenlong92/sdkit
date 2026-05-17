package ws

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
)

const websocketGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

func Upgrade(w http.ResponseWriter, r *http.Request, responseHeaders http.Header) (*Conn, error) {
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		return nil, errors.New("missing websocket upgrade")
	}
	if !strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade") {
		return nil, errors.New("missing upgrade connection")
	}
	key := r.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		return nil, errors.New("missing websocket key")
	}
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		return nil, errors.New("websocket hijack unsupported")
	}
	netConn, rw, err := hijacker.Hijack()
	if err != nil {
		return nil, err
	}
	headers := http.Header{}
	headers.Set("Upgrade", "websocket")
	headers.Set("Connection", "Upgrade")
	headers.Set("Sec-WebSocket-Accept", acceptKey(key))
	for key, values := range responseHeaders {
		for _, value := range values {
			headers.Add(key, value)
		}
	}
	if _, err := rw.WriteString("HTTP/1.1 101 Switching Protocols\r\n"); err != nil {
		_ = netConn.Close()
		return nil, err
	}
	if err := headers.Write(rw); err != nil {
		_ = netConn.Close()
		return nil, err
	}
	if _, err := rw.WriteString("\r\n"); err != nil {
		_ = netConn.Close()
		return nil, err
	}
	if err := rw.Flush(); err != nil {
		_ = netConn.Close()
		return nil, err
	}
	return NewConn(netConn, bufio.NewReader(rw)), nil
}

func acceptKey(key string) string {
	h := sha1.Sum([]byte(key + websocketGUID))
	return base64.StdEncoding.EncodeToString(h[:])
}
