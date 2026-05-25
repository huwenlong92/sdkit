package ipdb

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/lionsoul2014/ip2region/binding/golang/xdb"
)

type ip2RegionLocator struct {
	searcher *xdb.Searcher
	mu       sync.Mutex
	closed   bool
}

func newIP2RegionLocator(cfg Config) (Locator, error) {
	if strings.TrimSpace(cfg.Path) == "" {
		return nil, fmt.Errorf("%w: path is required", ErrInvalidConfig)
	}

	header, err := xdb.LoadHeaderFromFile(cfg.Path)
	if err != nil {
		return nil, err
	}
	version, err := xdb.VersionFromHeader(header)
	if err != nil {
		return nil, err
	}

	mode := strings.TrimSpace(cfg.Mode)
	if mode == "" {
		mode = IP2RegionModeVectorIndex
	}

	var searcher *xdb.Searcher
	switch mode {
	case IP2RegionModeFile:
		searcher, err = xdb.NewWithFileOnly(version, cfg.Path)
	case IP2RegionModeVectorIndex:
		var vectorIndex []byte
		vectorIndex, err = xdb.LoadVectorIndexFromFile(cfg.Path)
		if err == nil {
			searcher, err = xdb.NewWithVectorIndex(version, cfg.Path, vectorIndex)
		}
	case IP2RegionModeMemory:
		var content []byte
		content, err = xdb.LoadContentFromFile(cfg.Path)
		if err == nil {
			searcher, err = xdb.NewWithBuffer(version, content)
		}
	default:
		return nil, fmt.Errorf("%w: unsupported ip2region mode %s", ErrInvalidConfig, mode)
	}
	if err != nil {
		return nil, err
	}

	return &ip2RegionLocator{searcher: searcher}, nil
}

func (l *ip2RegionLocator) Lookup(ctx context.Context, rawIP string) (*Record, error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
	}

	ip, err := normalizeIP(rawIP)
	if err != nil {
		return nil, err
	}

	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		return nil, ErrClosed
	}
	region, err := l.searcher.Search(ip.String())
	l.mu.Unlock()
	if err != nil {
		return nil, err
	}
	if err := checkContext(ctx); err != nil {
		return nil, err
	}

	record := &Record{
		IP:      ip.String(),
		Version: ipVersion(ip),
		Source:  DriverIP2Region,
	}
	applyIP2Region(record, region)
	return record, nil
}

func (l *ip2RegionLocator) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return nil
	}
	l.closed = true
	if l.searcher != nil {
		l.searcher.Close()
	}
	return nil
}

func applyIP2Region(record *Record, region string) {
	parts := strings.Split(region, "|")
	normalize := func(i int) string {
		if i >= len(parts) {
			return ""
		}
		value := strings.TrimSpace(parts[i])
		if value == "0" {
			return ""
		}
		return value
	}

	record.Country = normalize(0)
	record.Region = normalize(2)
	record.Province = record.Region
	record.City = normalize(3)
	record.ISP = normalize(4)
}
