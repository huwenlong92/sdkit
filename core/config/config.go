package config

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

var V *viper.Viper

type Option func(*viper.Viper)

func Load(configPath string, out any, opts ...Option) error {
	v, err := New(configPath, opts...)
	if err != nil {
		return err
	}
	return v.Unmarshal(out)
}

func LoadKey(configPath string, key string, out any, opts ...Option) error {
	v, err := New(configPath, opts...)
	if err != nil {
		return err
	}
	return v.UnmarshalKey(key, out)
}

func LoadRequiredKey(configPath string, key string, out any, opts ...Option) error {
	v, err := New(configPath, opts...)
	if err != nil {
		return err
	}
	if !v.IsSet(key) {
		return fmt.Errorf("config key %q is required", key)
	}
	return v.UnmarshalKey(key, out)
}

func New(configPath string, opts ...Option) (*viper.Viper, error) {
	v := viper.New()
	v.SetConfigFile(configPath)
	v.SetConfigType("yaml")
	v.AutomaticEnv()
	v.SetEnvPrefix("SDKITGO")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	for _, opt := range opts {
		opt(v)
	}
	if err := v.ReadInConfig(); err != nil {
		return nil, err
	}
	if err := mergeImports(v, configPath); err != nil {
		return nil, err
	}
	V = v
	return v, nil
}

func mergeImports(v *viper.Viper, configPath string) error {
	imports := v.GetStringSlice("imports")
	if len(imports) == 0 {
		return nil
	}
	baseDir := filepath.Dir(configPath)
	for _, item := range imports {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		importPath := item
		if !filepath.IsAbs(importPath) {
			importPath = filepath.Join(baseDir, importPath)
		}
		v.SetConfigFile(importPath)
		if err := v.MergeInConfig(); err != nil {
			return fmt.Errorf("merge config %s: %w", importPath, err)
		}
	}
	v.SetConfigFile(configPath)
	return nil
}

func WithDefault(key string, value any) Option {
	return func(v *viper.Viper) {
		v.SetDefault(key, value)
	}
}

func WithDefaults(values map[string]any) Option {
	return func(v *viper.Viper) {
		for key, value := range values {
			v.SetDefault(key, value)
		}
	}
}
