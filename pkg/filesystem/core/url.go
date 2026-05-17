package core

import (
	"net/url"
	"path"
	"strings"
)

func JoinPublicURL(baseURL, objectPath string) string {
	if baseURL == "" {
		return ""
	}
	u, err := url.Parse(baseURL)
	if err != nil {
		return ""
	}
	u.Path = path.Join(u.Path, strings.TrimLeft(objectPath, "/"))
	return u.String()
}
