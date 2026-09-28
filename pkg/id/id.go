// Package id generates opaque, globally unique identifiers.
package id

import "github.com/segmentio/ksuid"

// New returns a new KSUID string.
func New() (string, error) {
	value, err := ksuid.NewRandom()
	if err != nil {
		return "", err
	}
	return value.String(), nil
}

// NewPrefixed returns a new KSUID string prefixed by prefix.
// Prefix semantics belong to the caller; this package does not maintain
// business identifier namespaces.
func NewPrefixed(prefix string) (string, error) {
	value, err := New()
	if err != nil {
		return "", err
	}
	return prefix + value, nil
}
