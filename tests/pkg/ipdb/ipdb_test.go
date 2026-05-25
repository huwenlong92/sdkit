package ipdb_test

import (
	"context"
	"errors"
	"testing"

	"github.com/huwenlong92/sdkit/pkg/ipdb"
)

func TestNewRejectsUnsupportedDriver(t *testing.T) {
	_, err := ipdb.New(ipdb.Config{Driver: "unknown"})
	if !errors.Is(err, ipdb.ErrUnsupportedDriver) {
		t.Fatalf("expected unsupported driver error, got %v", err)
	}
}

func TestNewIP2RegionRequiresPath(t *testing.T) {
	_, err := ipdb.New(ipdb.Config{Driver: ipdb.DriverIP2Region})
	if !errors.Is(err, ipdb.ErrInvalidConfig) {
		t.Fatalf("expected invalid config error, got %v", err)
	}
}

func TestNewMaxMindRequiresPath(t *testing.T) {
	_, err := ipdb.New(ipdb.Config{Driver: ipdb.DriverMaxMind})
	if !errors.Is(err, ipdb.ErrInvalidConfig) {
		t.Fatalf("expected invalid config error, got %v", err)
	}
}

func TestLookupHonorsInvalidIP(t *testing.T) {
	locator := fakeLocator{}
	_, err := locator.Lookup(context.Background(), "not-an-ip")
	if !errors.Is(err, ipdb.ErrInvalidIP) {
		t.Fatalf("expected invalid ip error, got %v", err)
	}
}

func TestRecordHasData(t *testing.T) {
	if (&ipdb.Record{IP: "127.0.0.1", Version: 4}).HasData() {
		t.Fatal("ip and version should not count as lookup data")
	}
	if !(&ipdb.Record{City: "上海"}).HasData() {
		t.Fatal("city should count as lookup data")
	}
	if !(&ipdb.Record{ASN: "AS15169"}).HasData() {
		t.Fatal("asn should count as lookup data")
	}
}

func TestHookFuncObservesLookup(t *testing.T) {
	var observed ipdb.LookupEvent
	hook := ipdb.HookFunc(func(ctx context.Context, event ipdb.LookupEvent) {
		observed = event
	})
	hook.AfterLookup(context.Background(), ipdb.LookupEvent{
		IP:  "127.0.0.1",
		Err: ipdb.ErrInvalidIP,
	})

	if observed.IP != "127.0.0.1" {
		t.Fatalf("unexpected observed ip %q", observed.IP)
	}
	if !errors.Is(observed.Err, ipdb.ErrInvalidIP) {
		t.Fatalf("unexpected observed error %v", observed.Err)
	}
}

type fakeLocator struct{}

func (fakeLocator) Lookup(ctx context.Context, rawIP string) (*ipdb.Record, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if rawIP != "127.0.0.1" {
		return nil, ipdb.ErrInvalidIP
	}
	return &ipdb.Record{IP: rawIP, Version: 4}, nil
}

func (fakeLocator) Close() error {
	return nil
}
