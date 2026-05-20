package core

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultSourceTTL  = 7 * 24 * time.Hour
	DefaultSourcePath = "/storage/source"
)

var (
	ErrSourceExpired          = errors.New("storage source url expired")
	ErrSourceSignatureInvalid = errors.New("storage source signature invalid")
	ErrSourceSecretRequired   = errors.New("storage source secret required")
)

func NormalizeSourceTTL(ttl time.Duration) time.Duration {
	if ttl > 0 {
		return ttl
	}
	return DefaultSourceTTL
}

func SignSourceURL(baseURL, objectPath, secret string, ttl time.Duration, now time.Time) (string, error) {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return "", ErrSourceSecretRequired
	}
	expires := now.Add(NormalizeSourceTTL(ttl)).Unix()
	signature := SourceSignature(objectPath, expires, secret)

	u, err := sourceBaseURL(baseURL)
	if err != nil {
		return "", err
	}
	query := u.Query()
	query.Set("path", objectPath)
	query.Set("expires", strconv.FormatInt(expires, 10))
	query.Set("signature", signature)
	u.RawQuery = query.Encode()
	return u.String(), nil
}

func VerifySourceSignature(objectPath, expires, signature, secret string, now time.Time) error {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return ErrSourceSecretRequired
	}
	expireUnix, err := strconv.ParseInt(expires, 10, 64)
	if err != nil {
		return ErrSourceSignatureInvalid
	}
	if now.Unix() > expireUnix {
		return ErrSourceExpired
	}
	expected := SourceSignature(objectPath, expireUnix, secret)
	if !hmac.Equal([]byte(signature), []byte(expected)) {
		return ErrSourceSignatureInvalid
	}
	return nil
}

func SourceSignature(objectPath string, expires int64, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(objectPath))
	mac.Write([]byte("\n"))
	mac.Write([]byte(strconv.FormatInt(expires, 10)))
	return hex.EncodeToString(mac.Sum(nil))
}

func sourceBaseURL(baseURL string) (*url.URL, error) {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return &url.URL{Path: DefaultSourcePath}, nil
	}
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}
	if u.Path == "" || strings.HasSuffix(u.Path, "/") {
		u.Path = path.Join(u.Path, strings.TrimLeft(DefaultSourcePath, "/"))
	}
	return u, nil
}
