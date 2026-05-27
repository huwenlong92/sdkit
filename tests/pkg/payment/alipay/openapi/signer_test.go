//go:build sdkit_payment_alipay

package openapi_test

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"testing"

	"github.com/huwenlong92/sdkit/pkg/payment/alipay/openapi"
)

func TestRSASignerFromPEM(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	raw := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})

	signer, err := openapi.NewRSASignerFromPEM(raw)
	if err != nil {
		t.Fatalf("new signer: %v", err)
	}
	content := "app_id=ali_app_1&method=alipay.trade.app.pay"
	signature, err := signer.Sign(context.Background(), content)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	decoded, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		t.Fatalf("decode signature: %v", err)
	}
	sum := sha256.Sum256([]byte(content))
	if err := rsa.VerifyPKCS1v15(&privateKey.PublicKey, crypto.SHA256, sum[:], decoded); err != nil {
		t.Fatalf("verify signature: %v", err)
	}
}

func TestRSASignerFromBareBase64Key(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	raw := base64.StdEncoding.EncodeToString(x509.MarshalPKCS1PrivateKey(privateKey))

	signer, err := openapi.NewRSASignerFromPEM([]byte(raw))
	if err != nil {
		t.Fatalf("new signer: %v", err)
	}
	if _, err := signer.Sign(context.Background(), "app_id=ali_app_1"); err != nil {
		t.Fatalf("sign: %v", err)
	}
}
