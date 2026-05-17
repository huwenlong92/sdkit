package sessionx

import (
	"net/http"
	"time"
)

type CookieOptions struct {
	Name     string
	Path     string
	Domain   string
	TTL      time.Duration
	Secure   bool
	HTTPOnly bool
	SameSite http.SameSite
}

func NewCookie(sessionID string, opts CookieOptions) *http.Cookie {
	if opts.Name == "" {
		opts.Name = "sid"
	}
	if opts.Path == "" {
		opts.Path = "/"
	}
	if opts.SameSite == 0 {
		opts.SameSite = http.SameSiteLaxMode
	}
	return &http.Cookie{
		Name:     opts.Name,
		Value:    sessionID,
		Path:     opts.Path,
		Domain:   opts.Domain,
		MaxAge:   int(opts.TTL.Seconds()),
		Secure:   opts.Secure,
		HttpOnly: opts.HTTPOnly,
		SameSite: opts.SameSite,
	}
}

func NewClearCookie(opts CookieOptions) *http.Cookie {
	cookie := NewCookie("", opts)
	cookie.MaxAge = -1
	return cookie
}
