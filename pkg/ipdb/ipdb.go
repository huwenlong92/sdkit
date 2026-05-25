// Package ipdb provides local IP database lookups.
package ipdb

import (
	"context"
	"fmt"
	"net/netip"
	"strings"
)

const (
	DriverIP2Region   = "ip2region"
	DriverMaxMind     = "maxmind"
	DriverMaxMindCity = "maxmind-city"
	DriverMaxMindASN  = "maxmind-asn"
)

const (
	IP2RegionModeFile        = "file"
	IP2RegionModeVectorIndex = "vector_index"
	IP2RegionModeMemory      = "memory"
)

const defaultLanguage = "zh-CN"

type Locator interface {
	Lookup(ctx context.Context, ip string) (*Record, error)
	Close() error
}

type Config struct {
	Driver string

	// Path is used by ip2region, maxmind-city and maxmind-asn.
	Path string

	// CityPath and ASNPath are used by DriverMaxMind to merge City and ASN records.
	CityPath string
	ASNPath  string

	// Language selects localized names from MaxMind databases.
	Language string

	// Mode controls ip2region loading strategy: file, vector_index or memory.
	Mode string

	// Hooks observe lookup results. Hook failures must be handled by the hook itself.
	Hooks []Hook
}

func New(cfg Config) (Locator, error) {
	driver := strings.TrimSpace(cfg.Driver)
	if driver == "" {
		driver = DriverIP2Region
	}

	var (
		locator Locator
		err     error
	)
	switch driver {
	case DriverIP2Region:
		locator, err = newIP2RegionLocator(cfg)
	case DriverMaxMindCity:
		locator, err = newMaxMindCityLocator(cfg)
	case DriverMaxMindASN:
		locator, err = newMaxMindASNLocator(cfg)
	case DriverMaxMind:
		locator, err = newMaxMindLocator(cfg)
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedDriver, driver)
	}
	if err != nil {
		return nil, err
	}
	if len(cfg.Hooks) > 0 {
		locator = &hookedLocator{next: locator, hooks: cfg.Hooks}
	}
	return locator, nil
}

func normalizeIP(raw string) (netip.Addr, error) {
	ip, err := netip.ParseAddr(strings.TrimSpace(raw))
	if err != nil {
		return netip.Addr{}, fmt.Errorf("%w: %s", ErrInvalidIP, raw)
	}
	return ip.Unmap(), nil
}

func ipVersion(ip netip.Addr) int {
	if ip.Is4() {
		return 4
	}
	if ip.Is6() {
		return 6
	}
	return 0
}

func checkContext(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}
