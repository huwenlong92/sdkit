// Package ffprobe implements media.Prober with a local ffprobe executable.
package ffprobe

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/huwenlong92/sdkit/pkg/execx"
	"github.com/huwenlong92/sdkit/pkg/media"
)

const defaultOutputLimit int64 = 8 * 1024 * 1024

var versionPattern = regexp.MustCompile(`(?m)^ffprobe version ([^\s]+)`)

type Config struct {
	BinaryPath  string `mapstructure:"binary_path" yaml:"binary_path"`
	MinVersion  string `mapstructure:"min_version" yaml:"min_version"`
	MaxVersion  string `mapstructure:"max_version" yaml:"max_version"`
	OutputLimit int64  `mapstructure:"output_limit" yaml:"output_limit"`
}

type Option func(*options)

type options struct {
	runner Runner
}

func WithRunner(runner Runner) Option {
	return func(options *options) {
		options.runner = runner
	}
}

type Driver struct {
	cfg     Config
	runner  Runner
	version string
}

func New(ctx context.Context, cfg Config, opts ...Option) (*Driver, error) {
	normalized, err := normalizeConfig(cfg)
	if err != nil {
		return nil, err
	}
	settings := options{runner: ExecRunner{}}
	for _, opt := range opts {
		if opt != nil {
			opt(&settings)
		}
	}
	if settings.runner == nil {
		return nil, invalidConfig("runner is required")
	}
	driver := &Driver{cfg: normalized, runner: settings.runner}
	version, err := driver.checkVersion(ctx)
	if err != nil {
		return nil, err
	}
	driver.version = version
	return driver, nil
}

func (d *Driver) Version() string {
	if d == nil {
		return ""
	}
	return d.version
}

func (d *Driver) Probe(ctx context.Context, input media.Input, opts media.ProbeOptions) (media.Info, error) {
	if d == nil || d.runner == nil {
		return media.Info{}, &media.Error{Operation: "probe", Err: media.ErrProbeFailed, Summary: "ffprobe driver is not initialized"}
	}
	ctx = normalizeContext(ctx)
	if err := ctx.Err(); err != nil {
		return media.Info{}, err
	}
	inputPath, err := validateInput(input.Path)
	if err != nil {
		return media.Info{}, err
	}
	limit := opts.OutputLimit
	if limit <= 0 {
		limit = d.cfg.OutputLimit
	}
	command := Command{
		Name: d.cfg.BinaryPath,
		Args: []string{
			"-v", "error",
			"-print_format", "json",
			"-show_format",
			"-show_streams",
			"-i", inputPath,
		},
		OutputLimit: limit,
	}
	result, runErr := d.runner.RunOutput(ctx, command)
	if runErr != nil {
		return media.Info{}, commandError("probe", runErr)
	}
	info, err := parseOutput(result.Stdout)
	if err != nil {
		return media.Info{}, &media.Error{Operation: "probe", Err: errors.Join(media.ErrOutputInvalid, err), Summary: "ffprobe returned invalid JSON metadata"}
	}
	return info, nil
}

func normalizeConfig(cfg Config) (Config, error) {
	cfg.BinaryPath = strings.TrimSpace(cfg.BinaryPath)
	cfg.MinVersion = strings.TrimSpace(cfg.MinVersion)
	cfg.MaxVersion = strings.TrimSpace(cfg.MaxVersion)
	if cfg.BinaryPath == "" {
		cfg.BinaryPath = "ffprobe"
	}
	if cfg.OutputLimit <= 0 {
		cfg.OutputLimit = defaultOutputLimit
	}
	if cfg.MinVersion != "" {
		if _, err := parseVersion(cfg.MinVersion); err != nil {
			return Config{}, invalidConfig("min_version is invalid")
		}
	}
	if cfg.MaxVersion != "" {
		if _, err := parseVersion(cfg.MaxVersion); err != nil {
			return Config{}, invalidConfig("max_version is invalid")
		}
	}
	if cfg.MinVersion != "" && cfg.MaxVersion != "" && compareVersions(cfg.MinVersion, cfg.MaxVersion) > 0 {
		return Config{}, invalidConfig("min_version must not exceed max_version")
	}
	return cfg, nil
}

func invalidConfig(summary string) error {
	return &media.Error{Operation: "open", Err: media.ErrInvalidInput, Summary: summary}
}

func (d *Driver) checkVersion(ctx context.Context) (string, error) {
	result, err := d.runner.RunOutput(normalizeContext(ctx), Command{Name: d.cfg.BinaryPath, Args: []string{"-version"}, OutputLimit: 64 * 1024})
	if err != nil {
		return "", commandError("open", err)
	}
	match := versionPattern.FindSubmatch(result.Stdout)
	if len(match) != 2 {
		return "", &media.Error{Operation: "open", Err: media.ErrVersionUnsupported, Summary: "ffprobe version output is unsupported"}
	}
	version := strings.TrimSpace(string(match[1]))
	if _, err := parseVersion(version); err != nil {
		if d.cfg.MinVersion != "" || d.cfg.MaxVersion != "" {
			return "", &media.Error{Operation: "open", Err: media.ErrVersionUnsupported, Summary: "ffprobe version is not comparable"}
		}
		return version, nil
	}
	if d.cfg.MinVersion != "" && compareVersions(version, d.cfg.MinVersion) < 0 {
		return "", &media.Error{Operation: "open", Err: media.ErrVersionUnsupported, Summary: "ffprobe version is below the configured minimum"}
	}
	if d.cfg.MaxVersion != "" && compareVersions(version, d.cfg.MaxVersion) > 0 {
		return "", &media.Error{Operation: "open", Err: media.ErrVersionUnsupported, Summary: "ffprobe version exceeds the configured maximum"}
	}
	return version, nil
}

func validateInput(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsRune(value, '\x00') || strings.Contains(value, "://") {
		return "", &media.Error{Operation: "probe", Err: media.ErrInvalidInput, Summary: "input must be a local file path"}
	}
	path, err := filepath.Abs(value)
	if err != nil {
		return "", &media.Error{Operation: "probe", Err: errors.Join(media.ErrInvalidInput, err), Summary: "input path could not be resolved"}
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", &media.Error{Operation: "probe", Err: errors.Join(media.ErrInputUnavailable, err), Summary: "input file is unavailable"}
	}
	if !info.Mode().IsRegular() {
		return "", &media.Error{Operation: "probe", Err: media.ErrInvalidInput, Summary: "input must be a regular file"}
	}
	return path, nil
}

func commandError(operation string, err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	if errors.Is(err, execx.ErrOutputLimitExceeded) {
		return &media.Error{Operation: operation, Err: media.ErrOutputLimit, Summary: "ffprobe output exceeded the configured limit"}
	}
	if errors.Is(err, exec.ErrNotFound) || errors.Is(err, os.ErrNotExist) {
		return &media.Error{Operation: operation, Err: media.ErrBinaryUnavailable, Summary: "ffprobe executable is unavailable"}
	}
	return &media.Error{Operation: operation, Err: media.ErrProbeFailed, Summary: "ffprobe command failed"}
}

func normalizeContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func parseVersion(value string) ([]int, error) {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "v") || strings.HasPrefix(value, "n") {
		value = value[1:]
	}
	parts := strings.SplitN(value, "-", 2)
	segments := strings.Split(parts[0], ".")
	if len(segments) == 0 || len(segments) > 4 {
		return nil, fmt.Errorf("invalid version")
	}
	parsed := make([]int, len(segments))
	for i, segment := range segments {
		if segment == "" {
			return nil, fmt.Errorf("invalid version")
		}
		number, err := strconv.Atoi(segment)
		if err != nil || number < 0 {
			return nil, fmt.Errorf("invalid version")
		}
		parsed[i] = number
	}
	return parsed, nil
}

func compareVersions(left string, right string) int {
	l, _ := parseVersion(left)
	r, _ := parseVersion(right)
	for len(l) < len(r) {
		l = append(l, 0)
	}
	for len(r) < len(l) {
		r = append(r, 0)
	}
	for i := range l {
		if l[i] < r[i] {
			return -1
		}
		if l[i] > r[i] {
			return 1
		}
	}
	return 0
}
