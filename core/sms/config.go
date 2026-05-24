package sms

import (
	"strings"

	pkgsms "github.com/huwenlong92/sdkit/pkg/sms"
)

const DefaultProviderName = "default"

type Config struct {
	Default   string                           `mapstructure:"default" yaml:"default"`
	Providers map[string]pkgsms.ProviderConfig `mapstructure:"providers" yaml:"providers"`
}

func (c Config) providerConfigs() (string, map[string]pkgsms.ProviderConfig) {
	providers := cloneProviderConfigs(c.Providers)
	if len(providers) == 0 {
		return "", nil
	}
	for name, cfg := range providers {
		cfg.Name = name
		providers[name] = cfg
	}
	return strings.TrimSpace(c.Default), providers
}

func cloneProviderConfigs(providers map[string]pkgsms.ProviderConfig) map[string]pkgsms.ProviderConfig {
	if len(providers) == 0 {
		return nil
	}
	clone := make(map[string]pkgsms.ProviderConfig, len(providers))
	for name, cfg := range providers {
		clone[name] = cfg.Clone()
	}
	return clone
}

func cleanProviderNames(names []string) []string {
	if len(names) == 0 {
		return nil
	}
	cleaned := make([]string, 0, len(names))
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		cleaned = append(cleaned, name)
	}
	return cleaned
}
