package sysprobe

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"

	"github.com/huwenlong92/sdkit/pkg/execx"
)

type DependencySpec struct {
	Key         string   `mapstructure:"key" yaml:"key"`
	Name        string   `mapstructure:"name" yaml:"name"`
	BinaryPath  string   `mapstructure:"binary_path" yaml:"binary_path"`
	Args        []string `mapstructure:"args" yaml:"args"`
	Env         []string `mapstructure:"env" yaml:"env"`
	PrepareDir  string   `mapstructure:"prepare_dir" yaml:"prepare_dir"`
	MinVersion  string   `mapstructure:"min_version" yaml:"min_version"`
	MaxVersion  string   `mapstructure:"max_version" yaml:"max_version"`
	Required    bool     `mapstructure:"required" yaml:"required"`
	Description string   `mapstructure:"description" yaml:"description"`
}

type DependencyCode string

const (
	DependencyReady          DependencyCode = "ready"
	DependencyMissing        DependencyCode = "missing"
	DependencyDirectoryError DependencyCode = "directory_error"
	DependencyCheckFailed    DependencyCode = "check_failed"
	DependencyVersionUnknown DependencyCode = "version_unknown"
	DependencyVersionOutside DependencyCode = "version_outside_range"
)

type DependencyStatus struct {
	Key         string         `json:"key"`
	Name        string         `json:"name"`
	Required    bool           `json:"required"`
	Installed   bool           `json:"installed"`
	Compatible  bool           `json:"compatible"`
	Version     string         `json:"version,omitempty"`
	Path        string         `json:"path,omitempty"`
	MinVersion  string         `json:"min_version,omitempty"`
	MaxVersion  string         `json:"max_version,omitempty"`
	Description string         `json:"description"`
	Code        DependencyCode `json:"code"`
	Message     string         `json:"message"`
}

type execRunner struct{}

func (execRunner) Run(ctx context.Context, spec DependencySpec) ([]byte, error) {
	options := []execx.Option{execx.WithOutputLimit(64 * 1024), execx.WithMergeStderr()}
	if len(spec.Env) > 0 {
		options = append(options, execx.WithEnv(spec.Env))
	}
	result, err := execx.RunOutput(ctx, spec.BinaryPath, spec.Args, options...)
	return result.Combined, err
}

func (s *Service) checkDependency(parent context.Context, spec DependencySpec) DependencyStatus {
	status := DependencyStatus{
		Key: spec.Key, Name: spec.Name, Required: spec.Required,
		MinVersion: spec.MinVersion, MaxVersion: spec.MaxVersion, Description: spec.Description,
	}
	resolved, err := exec.LookPath(spec.BinaryPath)
	if err != nil {
		status.Code, status.Message = DependencyMissing, "executable not found"
		return status
	}
	status.Installed, status.Path = true, resolved
	spec.BinaryPath = resolved
	if spec.PrepareDir != "" {
		directory, err := filepath.Abs(spec.PrepareDir)
		if err != nil {
			status.Code, status.Message = DependencyDirectoryError, "probe directory could not be resolved"
			return status
		}
		if err := os.MkdirAll(directory, 0700); err != nil {
			status.Code, status.Message = DependencyDirectoryError, "probe directory is not writable"
			return status
		}
	}
	ctx, cancel := context.WithTimeout(parent, s.config.CheckTimeout)
	defer cancel()
	output, err := s.runner.Run(ctx, spec)
	if err != nil {
		status.Code, status.Message = DependencyCheckFailed, "version check failed"
		return status
	}
	version, parsed, err := parseSemanticVersion(string(output))
	if err != nil {
		status.Code, status.Message = DependencyVersionUnknown, "version could not be identified"
		return status
	}
	status.Version = version
	if !versionInRange(parsed, spec.MinVersion, spec.MaxVersion) {
		status.Code, status.Message = DependencyVersionOutside, "version is outside the configured range"
		return status
	}
	status.Compatible, status.Code, status.Message = true, DependencyReady, "installed and compatible"
	return status
}

var semanticVersionPattern = regexp.MustCompile(`(?i)\b[nv]?(\d+)\.(\d+)\.(\d+)\b`)

type semanticVersion struct{ major, minor, patch int }

func parseSemanticVersion(value string) (string, semanticVersion, error) {
	match := semanticVersionPattern.FindStringSubmatch(value)
	if len(match) != 4 {
		return "", semanticVersion{}, fmt.Errorf("sysprobe: semantic version is unavailable")
	}
	parts := [3]int{}
	for i := range parts {
		parsed, err := strconv.Atoi(match[i+1])
		if err != nil {
			return "", semanticVersion{}, err
		}
		parts[i] = parsed
	}
	return fmt.Sprintf("%d.%d.%d", parts[0], parts[1], parts[2]), semanticVersion{parts[0], parts[1], parts[2]}, nil
}

func versionInRange(current semanticVersion, minimum, maximum string) bool {
	if minimum != "" {
		_, limit, err := parseSemanticVersion(minimum)
		if err != nil || current.compare(limit) < 0 {
			return false
		}
	}
	if maximum != "" {
		_, limit, err := parseSemanticVersion(maximum)
		if err != nil || current.compare(limit) > 0 {
			return false
		}
	}
	return true
}

func (v semanticVersion) compare(other semanticVersion) int {
	left, right := [...]int{v.major, v.minor, v.patch}, [...]int{other.major, other.minor, other.patch}
	for i := range left {
		if left[i] < right[i] {
			return -1
		}
		if left[i] > right[i] {
			return 1
		}
	}
	return 0
}
