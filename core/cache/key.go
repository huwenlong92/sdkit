package cache

import (
	"fmt"
	"strings"
)

// Key joins cache key parts with ":" and trims accidental separators.
func Key(parts ...any) string {
	keyParts := make([]string, 0, len(parts))
	for _, part := range parts {
		s := strings.Trim(fmt.Sprint(part), ":")
		if s != "" {
			keyParts = append(keyParts, s)
		}
	}
	return strings.Join(keyParts, ":")
}

// UserKey returns the conventional cache key for a user.
func UserKey(id any) string {
	return Key("user", id)
}

// SessionKey returns the conventional cache key for a session.
func SessionKey(id any) string {
	return Key("session", id)
}
