package fingerprint

import (
	"net"
	"net/http"
	"strings"
)

type RequestInfo struct {
	IP       string
	UA       string
	DeviceID string
	Method   string
	Path     string
}

func FromRequest(r *http.Request) RequestInfo {
	ip := r.Header.Get("X-Forwarded-For")
	if ip != "" {
		ip = strings.TrimSpace(strings.Split(ip, ",")[0])
	}
	if ip == "" {
		ip = r.Header.Get("X-Real-IP")
	}
	if ip == "" {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err == nil {
			ip = host
		} else {
			ip = r.RemoteAddr
		}
	}
	return RequestInfo{
		IP:       ip,
		UA:       r.UserAgent(),
		DeviceID: r.Header.Get("X-Device-ID"),
		Method:   r.Method,
		Path:     r.URL.Path,
	}
}
