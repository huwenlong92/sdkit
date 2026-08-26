package offsetcursor

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
)

var ErrInvalid = errors.New("offset cursor is invalid for this query")

type token struct {
	Offset int    `json:"o"`
	Scope  string `json:"s"`
}

func Scope(parts ...string) string {
	hash := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(hash[:16])
}

func Encode(scope string, offset int) string {
	data, _ := json.Marshal(token{Offset: offset, Scope: scope})
	return base64.RawURLEncoding.EncodeToString(data)
}

func Decode(value string, scope string, length int) (int, error) {
	if value == "" {
		return 0, nil
	}
	data, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return 0, ErrInvalid
	}
	var decoded token
	if err := json.Unmarshal(data, &decoded); err != nil || decoded.Scope != scope || decoded.Offset < 0 || decoded.Offset > length {
		return 0, ErrInvalid
	}
	return decoded.Offset, nil
}
