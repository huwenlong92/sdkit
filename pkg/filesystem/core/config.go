package core

import "time"

type Config struct {
	Driver            string        `mapstructure:"driver"`
	UploadDir         string        `mapstructure:"upload_dir"`
	TempDir           string        `mapstructure:"temp_dir"`
	MaxSize           int64         `mapstructure:"max_size"`
	ChunkSize         int64         `mapstructure:"chunk_size"`
	TokenTTL          time.Duration `mapstructure:"token_ttl"`
	DirRule           string        `mapstructure:"dir_rule"`
	FileRule          string        `mapstructure:"file_rule"`
	AllowedExtensions []string      `mapstructure:"allowed_extensions"`
	Policy            StoragePolicy `mapstructure:"policy"`
	DriverConfig      DriverConfig  `mapstructure:",remain"`
}

type DriverConfig map[string]map[string]any

// StoragePolicy is a unified storage strategy shape for config files or DB rows.
// Driver decides how these common fields are mapped to a concrete SDK config.
type StoragePolicy struct {
	Driver        string `mapstructure:"driver" json:"driver"`
	Name          string `mapstructure:"name" json:"name"`
	Bucket        string `mapstructure:"bucket" json:"bucket"`
	Endpoint      string `mapstructure:"endpoint" json:"endpoint"`
	EndpointInner string `mapstructure:"endpoint_inner" json:"endpoint_inner"`
	PublicURL     string `mapstructure:"public_url" json:"public_url"`
	CDNURL        string `mapstructure:"cdn_url" json:"cdn_url"`
	Region        string `mapstructure:"region" json:"region"`
	AccessKey     string `mapstructure:"access_key" json:"access_key"`
	SecretKey     string `mapstructure:"secret_key" json:"secret_key"`
	UseSSL        bool   `mapstructure:"use_ssl" json:"use_ssl"`
	LocalDir      string `mapstructure:"local_dir" json:"local_dir"`
}

func (p StoragePolicy) Empty() bool {
	return p == StoragePolicy{}
}

func (c Config) Clone() Config {
	next := c
	next.AllowedExtensions = append([]string(nil), c.AllowedExtensions...)
	if c.DriverConfig != nil {
		next.DriverConfig = make(DriverConfig, len(c.DriverConfig))
		for driver, values := range c.DriverConfig {
			clone := make(map[string]any, len(values))
			for key, value := range values {
				clone[key] = value
			}
			next.DriverConfig[driver] = clone
		}
	}
	return next
}

func (c Config) DriverString(driver, key string) string {
	if c.DriverConfig == nil {
		return ""
	}
	values := c.DriverConfig[driver]
	if values == nil {
		return ""
	}
	value, ok := values[key]
	if !ok || value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}

func (c Config) DriverBool(driver, key string) bool {
	if c.DriverConfig == nil {
		return false
	}
	values := c.DriverConfig[driver]
	if values == nil {
		return false
	}
	value, ok := values[key]
	if !ok || value == nil {
		return false
	}
	if boolean, ok := value.(bool); ok {
		return boolean
	}
	return false
}
