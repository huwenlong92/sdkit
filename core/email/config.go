package email

import (
	"strings"

	pkgemail "github.com/huwenlong92/sdkit/pkg/email"
)

const DefaultProviderName = "default"

type Config struct {
	Default   string                             `mapstructure:"default" yaml:"default"`
	Fallback  []string                           `mapstructure:"fallback" yaml:"fallback"`
	Providers map[string]pkgemail.ProviderConfig `mapstructure:"providers" yaml:"providers"`
	Templates map[string]pkgemail.Template       `mapstructure:"templates" yaml:"templates"`
}

func (c Config) providerConfigs() (string, []string, map[string]pkgemail.ProviderConfig) {
	providers := cloneProviderConfigs(c.Providers)
	if len(providers) == 0 {
		return "", nil, nil
	}
	return strings.TrimSpace(c.Default), cleanProviderNames(c.Fallback), providers
}

func cloneProviderConfigs(providers map[string]pkgemail.ProviderConfig) map[string]pkgemail.ProviderConfig {
	if len(providers) == 0 {
		return nil
	}
	clone := make(map[string]pkgemail.ProviderConfig, len(providers))
	for name, cfg := range providers {
		clone[name] = cfg.Clone()
	}
	return clone
}

func cloneTemplates(templates map[string]pkgemail.Template) map[string]pkgemail.Template {
	if len(templates) == 0 {
		return nil
	}
	clone := make(map[string]pkgemail.Template, len(templates))
	for name, tpl := range templates {
		clone[name] = tpl
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
