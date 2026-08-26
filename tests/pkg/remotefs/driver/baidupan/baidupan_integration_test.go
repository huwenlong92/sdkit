package baidupan_test

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/huwenlong92/sdkit/pkg/remotefs"
	"github.com/huwenlong92/sdkit/pkg/remotefs/driver/baidupan"
)

func TestBaiduPanIntegration(t *testing.T) {
	if os.Getenv("SDKIT_BAIDUPAN_INTEGRATION") != "1" {
		t.Skip("set SDKIT_BAIDUPAN_INTEGRATION=1 to run the BaiduPCS-Go integration test")
	}
	binary := os.Getenv("SDKIT_BAIDUPAN_BINARY")
	if binary == "" {
		var err error
		binary, err = exec.LookPath("BaiduPCS-Go")
		if err != nil {
			t.Skip("BaiduPCS-Go is not installed")
		}
	}
	account := os.Getenv("SDKIT_BAIDUPAN_ACCOUNT")
	if account == "" {
		t.Skip("set SDKIT_BAIDUPAN_ACCOUNT to an isolated test account name")
	}
	minVersion := os.Getenv("SDKIT_BAIDUPAN_MIN_VERSION")
	if minVersion == "" {
		minVersion = "v4.0.0"
	}
	maxVersion := os.Getenv("SDKIT_BAIDUPAN_MAX_VERSION")
	if maxVersion == "" {
		maxVersion = "v4.0.0"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	runtime, err := baidupan.NewRuntime(ctx, baidupan.RuntimeConfig{
		BinaryPath:  binary,
		MinVersion:  minVersion,
		MaxVersion:  maxVersion,
		ConfigRoot:  t.TempDir(),
		OutputLimit: 1024 * 1024,
	})
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}
	fs, err := runtime.Open(ctx, baidupan.SessionConfig{
		SessionKey: account,
		BDUSS:      os.Getenv("SDKIT_BAIDUPAN_BDUSS"),
		STOKEN:     os.Getenv("SDKIT_BAIDUPAN_STOKEN"),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() { _ = fs.Close() })
	checker, ok := fs.(remotefs.HealthChecker)
	if !ok {
		t.Fatal("baidupan does not implement HealthChecker")
	}
	health, err := checker.Check(ctx)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if health.Status != remotefs.HealthHealthy {
		t.Fatalf("health = %+v", health)
	}
}
