package ipdb

import (
	"context"
	"fmt"
	"net/netip"
	"strings"
	"sync"

	geoip2 "github.com/oschwald/geoip2-golang/v2"
)

type maxMindKind int

const (
	maxMindKindCity maxMindKind = iota + 1
	maxMindKindASN
)

type maxMindLocator struct {
	reader   *geoip2.Reader
	kind     maxMindKind
	language string
	mu       sync.RWMutex
	closed   bool
}

func newMaxMindCityLocator(cfg Config) (Locator, error) {
	path := strings.TrimSpace(cfg.Path)
	if path == "" {
		path = strings.TrimSpace(cfg.CityPath)
	}
	return newSingleMaxMindLocator(path, maxMindKindCity, cfg.Language)
}

func newMaxMindASNLocator(cfg Config) (Locator, error) {
	path := strings.TrimSpace(cfg.Path)
	if path == "" {
		path = strings.TrimSpace(cfg.ASNPath)
	}
	return newSingleMaxMindLocator(path, maxMindKindASN, cfg.Language)
}

func newSingleMaxMindLocator(path string, kind maxMindKind, language string) (Locator, error) {
	if path == "" {
		return nil, fmt.Errorf("%w: path is required", ErrInvalidConfig)
	}
	reader, err := geoip2.Open(path)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(language) == "" {
		language = defaultLanguage
	}
	return &maxMindLocator{reader: reader, kind: kind, language: language}, nil
}

func (l *maxMindLocator) Lookup(ctx context.Context, rawIP string) (*Record, error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
	}

	ip, err := normalizeIP(rawIP)
	if err != nil {
		return nil, err
	}

	l.mu.RLock()
	if l.closed {
		l.mu.RUnlock()
		return nil, ErrClosed
	}
	reader := l.reader
	l.mu.RUnlock()

	record := &Record{
		IP:      ip.String(),
		Version: ipVersion(ip),
	}
	switch l.kind {
	case maxMindKindCity:
		record.Source = DriverMaxMindCity
		err = l.lookupCity(reader, ip, record)
	case maxMindKindASN:
		record.Source = DriverMaxMindASN
		err = l.lookupASN(reader, ip, record)
	default:
		err = fmt.Errorf("%w: invalid maxmind locator kind", ErrInvalidConfig)
	}
	if err != nil {
		return nil, err
	}
	if err := checkContext(ctx); err != nil {
		return nil, err
	}
	return record, nil
}

func (l *maxMindLocator) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return nil
	}
	l.closed = true
	if l.reader != nil {
		return l.reader.Close()
	}
	return nil
}

func (l *maxMindLocator) lookupCity(reader *geoip2.Reader, ip netip.Addr, record *Record) error {
	city, err := reader.City(ip)
	if err != nil {
		return err
	}

	record.Continent = localizedName(city.Continent.Names, l.language)
	record.ContinentCode = city.Continent.Code
	record.Country = localizedName(city.Country.Names, l.language)
	record.CountryCode = city.Country.ISOCode
	record.RegisteredCountry = localizedName(city.RegisteredCountry.Names, l.language)
	record.RegisteredCountryCode = city.RegisteredCountry.ISOCode
	record.City = localizedName(city.City.Names, l.language)
	record.PostalCode = city.Postal.Code
	record.TimeZone = city.Location.TimeZone
	if city.Traits.Network.IsValid() {
		record.Network = city.Traits.Network.String()
	}

	if len(city.Subdivisions) > 0 {
		subdivision := city.Subdivisions[0]
		record.Region = localizedName(subdivision.Names, l.language)
		record.RegionCode = subdivision.ISOCode
		record.Province = record.Region
	}
	if city.Location.Latitude != nil {
		lat := *city.Location.Latitude
		record.Latitude = &lat
	}
	if city.Location.Longitude != nil {
		lon := *city.Location.Longitude
		record.Longitude = &lon
	}
	return nil
}

func (l *maxMindLocator) lookupASN(reader *geoip2.Reader, ip netip.Addr, record *Record) error {
	asn, err := reader.ASN(ip)
	if err != nil {
		return err
	}
	record.ASNumber = asn.AutonomousSystemNumber
	record.ASOrganization = asn.AutonomousSystemOrganization
	if asn.AutonomousSystemNumber != 0 {
		record.ASN = fmt.Sprintf("AS%d", asn.AutonomousSystemNumber)
	}
	if asn.Network.IsValid() {
		record.Network = asn.Network.String()
	}
	return nil
}

func localizedName(names geoip2.Names, language string) string {
	for _, key := range []string{language, defaultLanguage, "zh-CN", "en"} {
		if value := strings.TrimSpace(nameByLanguage(names, key)); value != "" {
			return value
		}
	}
	for _, value := range []string{
		names.English,
		names.SimplifiedChinese,
		names.Japanese,
		names.French,
		names.Spanish,
		names.German,
		names.Russian,
		names.BrazilianPortuguese,
	} {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func nameByLanguage(names geoip2.Names, language string) string {
	switch strings.TrimSpace(language) {
	case "de":
		return names.German
	case "en":
		return names.English
	case "es":
		return names.Spanish
	case "fr":
		return names.French
	case "ja":
		return names.Japanese
	case "pt-BR":
		return names.BrazilianPortuguese
	case "ru":
		return names.Russian
	case "zh-CN":
		return names.SimplifiedChinese
	}
	return ""
}
