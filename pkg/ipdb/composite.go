package ipdb

import (
	"context"
	"fmt"
	"strings"
)

type compositeLocator struct {
	locators []Locator
}

func newMaxMindLocator(cfg Config) (Locator, error) {
	var locators []Locator
	if strings.TrimSpace(cfg.CityPath) != "" {
		locator, err := newMaxMindCityLocator(Config{
			Path:     cfg.CityPath,
			Language: cfg.Language,
		})
		if err != nil {
			return nil, err
		}
		locators = append(locators, locator)
	}
	if strings.TrimSpace(cfg.ASNPath) != "" {
		locator, err := newMaxMindASNLocator(Config{
			Path:     cfg.ASNPath,
			Language: cfg.Language,
		})
		if err != nil {
			closeLocators(locators)
			return nil, err
		}
		locators = append(locators, locator)
	}
	if len(locators) == 0 {
		return nil, fmt.Errorf("%w: city_path or asn_path is required", ErrInvalidConfig)
	}
	return &compositeLocator{locators: locators}, nil
}

func (l *compositeLocator) Lookup(ctx context.Context, ip string) (*Record, error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
	}
	var merged *Record
	for _, locator := range l.locators {
		record, err := locator.Lookup(ctx, ip)
		if err != nil {
			return nil, err
		}
		merged = mergeRecord(merged, record)
	}
	return merged, checkContext(ctx)
}

func (l *compositeLocator) Close() error {
	return closeLocators(l.locators)
}

func closeLocators(locators []Locator) error {
	var closeErr error
	for _, locator := range locators {
		if locator == nil {
			continue
		}
		if err := locator.Close(); err != nil && closeErr == nil {
			closeErr = err
		}
	}
	return closeErr
}
