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
	if err := mergeImports(v, configPath, opts...); err != nil {
		return nil, err
	}
	V = v
	return v, nil
}

func mergeImports(v *viper.Viper, configPath string, opts ...Option) error {
	imports := v.GetStringSlice("imports")
	if len(imports) == 0 {
		return nil
	}
	rootIdentity, err := importIdentity(configPath)
	if err != nil {
		return err
	}
	active := map[string]bool{rootIdentity: true}
	// Keep ConfigFileUsed pointing at the entrypoint, including on import failure.
	defer v.SetConfigFile(configPath)
	return mergeImportFiles(v, configPath, imports, active, []string{configPath}, opts)
}

func mergeImportFiles(v *viper.Viper, parentPath string, imports []string, active map[string]bool, chain []string, opts []Option) error {
	for _, item := range imports {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		importPath := item
		if !filepath.IsAbs(importPath) {
			importPath = filepath.Join(filepath.Dir(parentPath), importPath)
		}
		nextChain := append(append([]string(nil), chain...), importPath)
		identity, err := importIdentity(importPath)
		if err != nil {
			return fmt.Errorf("config import chain %s: %w", strings.Join(nextChain, " -> "), err)
		}
		if active[identity] {
			return fmt.Errorf("config import cycle: %s", strings.Join(nextChain, " -> "))
		}

		// Read each file's own imports; merged imports may belong to an earlier file.
		// Replay options so custom filesystems and decoders work for child readers too.
		child := viper.New()
		child.SetConfigType("yaml")
		for _, opt := range opts {
			opt(child)
		}
		child.SetConfigFile(importPath)
		if err := child.ReadInConfig(); err != nil {
			return fmt.Errorf("config import chain %s: %w", strings.Join(nextChain, " -> "), err)
		}
		v.SetConfigFile(importPath)
		if err := v.MergeInConfig(); err != nil {
			return fmt.Errorf("config import chain %s: %w", strings.Join(nextChain, " -> "), err)
		}
		var childImports []string
		if child.InConfig("imports") {
			childImports = child.GetStringSlice("imports")
		}
		active[identity] = true
		err = mergeImportFiles(v, importPath, childImports, active, nextChain, opts)
		delete(active, identity)
		if err != nil {
			return err
		}
	}
	return nil
}

func importIdentity(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	// Resolve symlink aliases for cycle detection. Virtual filesystem paths may
	// not exist on the host; their cleaned absolute paths still identify cycles.
	if resolved, err := filepath.EvalSymlinks(absolute); err == nil {
		return resolved, nil
	}
	return absolute, nil
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
