package storage

import (
	"strings"
	"time"

	pkgfs "github.com/huwenlong92/sdkit/pkg/storage"
	"github.com/huwenlong92/sdkit/pkg/storage/core"
)

type FileSystem = pkgfs.FileSystem
type Policy = core.StoragePolicy
type Option = pkgfs.Option
type DriverConfig = core.DriverConfig

const (
	DefaultStoreName = "default"
)

type StoreConfig struct {
	Driver            string        `mapstructure:"driver" yaml:"driver"`
	TempDir           string        `mapstructure:"temp_dir" yaml:"temp_dir"`
	MaxSize           int64         `mapstructure:"max_size" yaml:"max_size"`
	ChunkSize         int64         `mapstructure:"chunk_size" yaml:"chunk_size"`
	TokenTTL          time.Duration `mapstructure:"token_ttl" yaml:"token_ttl"`
	DirRule           string        `mapstructure:"dir_rule" yaml:"dir_rule"`
	FileRule          string        `mapstructure:"file_rule" yaml:"file_rule"`
	AllowedExtensions []string      `mapstructure:"allowed_extensions" yaml:"allowed_extensions"`

	Name          string `mapstructure:"name" yaml:"name"`
	Bucket        string `mapstructure:"bucket" yaml:"bucket"`
	Endpoint      string `mapstructure:"endpoint" yaml:"endpoint"`
	EndpointInner string `mapstructure:"endpoint_inner" yaml:"endpoint_inner"`
	CDNURL        string `mapstructure:"cdn_url" yaml:"cdn_url"`
	Region        string `mapstructure:"region" yaml:"region"`
	AccessKey     string `mapstructure:"access_key" yaml:"access_key"`
	SecretKey     string `mapstructure:"secret_key" yaml:"secret_key"`
	SecretID      string `mapstructure:"secret_id" yaml:"secret_id"`
	AccessKeyID   string `mapstructure:"access_key_id" yaml:"access_key_id"`
	AccessSecret  string `mapstructure:"access_secret" yaml:"access_secret"`
	Dir           string `mapstructure:"dir" yaml:"dir"`
	LocalDir      string `mapstructure:"local_dir" yaml:"local_dir"`

	Policy       Policy            `mapstructure:"policy" yaml:"policy"`
	DriverConfig core.DriverConfig `mapstructure:",remain" yaml:",inline"`
}

type Config struct {
	Default string                 `mapstructure:"default" yaml:"default"`
	Stores  map[string]StoreConfig `mapstructure:"stores" yaml:"stores"`
}

func (c Config) storeConfigs() (string, map[string]StoreConfig) {
	stores := cloneStores(c.Stores)
	if len(stores) == 0 {
		return "", nil
	}

	defaultName := strings.TrimSpace(c.Default)
	return defaultName, stores
}

func (c StoreConfig) Clone() StoreConfig {
	next := c
	next.AllowedExtensions = append([]string(nil), c.AllowedExtensions...)
	next.DriverConfig = cloneDriverConfig(c.DriverConfig)
	return next
}

func (c StoreConfig) storageConfig() core.Config {
	policy := c.Policy
	policy.Driver = firstNonEmpty(policy.Driver, c.Driver)
	policy.Name = firstNonEmpty(policy.Name, c.Name)
	policy.Bucket = firstNonEmpty(policy.Bucket, c.Bucket)
	policy.Endpoint = firstNonEmpty(policy.Endpoint, c.Endpoint)
	policy.EndpointInner = firstNonEmpty(policy.EndpointInner, c.EndpointInner)
	policy.CDNURL = firstNonEmpty(policy.CDNURL, c.CDNURL)
	policy.Region = firstNonEmpty(policy.Region, c.Region)
	policy.AccessKey = firstNonEmpty(policy.AccessKey, c.AccessKey, c.SecretID, c.AccessKeyID)
	policy.SecretKey = firstNonEmpty(policy.SecretKey, c.SecretKey, c.AccessSecret)
	policy.LocalDir = firstNonEmpty(policy.LocalDir, c.LocalDir, c.Dir)

	return core.Config{
		Driver:            firstNonEmpty(c.Driver, policy.Driver),
		TempDir:           c.TempDir,
		MaxSize:           c.MaxSize,
		ChunkSize:         c.ChunkSize,
		TokenTTL:          c.TokenTTL,
		DirRule:           c.DirRule,
		FileRule:          c.FileRule,
		AllowedExtensions: append([]string(nil), c.AllowedExtensions...),
		Policy:            policy,
		DriverConfig:      cloneDriverConfig(c.DriverConfig),
	}
}

func cloneStores(stores map[string]StoreConfig) map[string]StoreConfig {
	if len(stores) == 0 {
		return nil
	}
	clone := make(map[string]StoreConfig, len(stores))
	for name, cfg := range stores {
		clone[name] = cfg.Clone()
	}
	return clone
}

func cloneDriverConfig(cfg core.DriverConfig) core.DriverConfig {
	if cfg == nil {
		return nil
	}
	clone := make(core.DriverConfig, len(cfg))
	for driver, values := range cfg {
		next := make(map[string]any, len(values))
		for key, value := range values {
			next[key] = value
		}
		clone[driver] = next
	}
	return clone
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
