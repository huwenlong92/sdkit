package baidupan

import (
	"path/filepath"
	"strings"

	"github.com/huwenlong92/sdkit/pkg/remotefs"
)

const (
	DriverName         = "baidupan"
	defaultOutputLimit = 10 * 1024 * 1024
	maxOutputLimit     = 64 * 1024 * 1024
	maxConcurrency     = 32
	defaultPageSize    = 100
	maxPageSize        = 1000
)

type RuntimeConfig struct {
	BinaryPath  string `mapstructure:"binary_path" yaml:"binary_path"`
	MinVersion  string `mapstructure:"min_version" yaml:"min_version"`
	MaxVersion  string `mapstructure:"max_version" yaml:"max_version"`
	ConfigRoot  string `mapstructure:"config_root" yaml:"config_root"`
	OutputLimit int64  `mapstructure:"output_limit" yaml:"output_limit"`
}

type SessionConfig struct {
	SessionKey          string `mapstructure:"session_key" yaml:"session_key"`
	ExpectedProviderUID string `mapstructure:"expected_provider_uid" yaml:"expected_provider_uid"`
	CredentialRevision  string `mapstructure:"credential_revision" yaml:"credential_revision"`
	BDUSS               string `mapstructure:"bduss" yaml:"bduss"`
	STOKEN              string `mapstructure:"stoken" yaml:"stoken"`
	DownloadConcurrency int    `mapstructure:"download_concurrency" yaml:"download_concurrency"`
	DownloadMode        string `mapstructure:"download_mode" yaml:"download_mode"`
	AllowRemove         bool   `mapstructure:"allow_remove" yaml:"allow_remove"`
}

func (c RuntimeConfig) normalized() (RuntimeConfig, error) {
	c.BinaryPath = strings.TrimSpace(c.BinaryPath)
	c.MinVersion = strings.TrimSpace(c.MinVersion)
	c.MaxVersion = strings.TrimSpace(c.MaxVersion)
	c.ConfigRoot = strings.TrimSpace(c.ConfigRoot)
	if c.BinaryPath == "" {
		return RuntimeConfig{}, invalidConfig("binary_path is required")
	}
	if !filepath.IsAbs(c.BinaryPath) {
		return RuntimeConfig{}, invalidConfig("binary_path must be absolute")
	}
	if c.MinVersion == "" || c.MaxVersion == "" {
		return RuntimeConfig{}, invalidConfig("min_version and max_version are required")
	}
	if _, err := parseVersion(c.MinVersion); err != nil {
		return RuntimeConfig{}, invalidConfig("min_version is invalid")
	}
	if _, err := parseVersion(c.MaxVersion); err != nil {
		return RuntimeConfig{}, invalidConfig("max_version is invalid")
	}
	if compareVersionStrings(c.MinVersion, c.MaxVersion) > 0 {
		return RuntimeConfig{}, invalidConfig("min_version must not exceed max_version")
	}
	if c.ConfigRoot == "" {
		return RuntimeConfig{}, invalidConfig("config_root is required")
	}
	if c.OutputLimit <= 0 {
		c.OutputLimit = defaultOutputLimit
	}
	if c.OutputLimit > maxOutputLimit {
		return RuntimeConfig{}, invalidConfig("output_limit exceeds the supported maximum")
	}
	return c, nil
}

func (c SessionConfig) normalized() (SessionConfig, error) {
	c.SessionKey = strings.TrimSpace(c.SessionKey)
	c.ExpectedProviderUID = strings.TrimSpace(c.ExpectedProviderUID)
	c.CredentialRevision = strings.TrimSpace(c.CredentialRevision)
	c.DownloadMode = strings.ToLower(strings.TrimSpace(c.DownloadMode))
	if c.SessionKey == "" {
		return SessionConfig{}, invalidConfig("session_key is required")
	}
	if c.DownloadConcurrency <= 0 {
		c.DownloadConcurrency = 1
	}
	if c.DownloadConcurrency > maxConcurrency {
		return SessionConfig{}, invalidConfig("download_concurrency exceeds the supported maximum")
	}
	if c.DownloadMode == "" {
		c.DownloadMode = "locate"
	}
	switch c.DownloadMode {
	case "locate", "pcs", "stream":
	default:
		return SessionConfig{}, invalidConfig("download_mode must be locate, pcs, or stream")
	}
	return c, nil
}

func invalidConfig(summary string) error {
	return remotefs.WrapError("open", DriverName, remotefs.ErrInvalidArgument, summary)
}
