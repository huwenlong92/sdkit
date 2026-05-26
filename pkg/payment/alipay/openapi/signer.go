package openapi

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"strings"

	"github.com/huwenlong92/sdkit/core/payment"
)

type RSASigner struct {
	privateKey *rsa.PrivateKey
}

func NewRSASigner(privateKey *rsa.PrivateKey) (*RSASigner, error) {
	if privateKey == nil {
		return nil, fmt.Errorf("%w: alipay private key is required", payment.ErrInvalidRequest)
	}
	return &RSASigner{privateKey: privateKey}, nil
}

func NewRSASignerFromPEM(raw []byte) (*RSASigner, error) {
	privateKey, err := parsePrivateKeyPEM(raw)
	if err != nil {
		return nil, err
	}
	return NewRSASigner(privateKey)
}

func (s *RSASigner) Sign(ctx context.Context, content string) (string, error) {
	if ctx == nil {
		return "", payment.ErrNilContext
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(content))
	signature, err := rsa.SignPKCS1v15(rand.Reader, s.privateKey, crypto.SHA256, sum[:])
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(signature), nil
}

func parsePrivateKeyPEM(raw []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(raw)
	if block == nil {
		decoded, err := base64.StdEncoding.DecodeString(compactKey(raw))
		if err != nil {
			return nil, fmt.Errorf("%w: invalid private key pem", payment.ErrInvalidRequest)
		}
		block = &pem.Block{Bytes: decoded}
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("%w: parse private key: %v", payment.ErrInvalidRequest, err)
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("%w: private key is not rsa", payment.ErrInvalidRequest)
	}
	return key, nil
}

func compactKey(raw []byte) string {
	text := strings.TrimSpace(string(raw))
	text = strings.ReplaceAll(text, "\r", "")
	text = strings.ReplaceAll(text, "\n", "")
	text = strings.ReplaceAll(text, " ", "")
	return text
}
