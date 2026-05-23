package execx

import (
	"io"
	"strings"
	"time"
)

const (
	defaultLineBufferSize = 1024 * 1024
	defaultOutputLimit    = 10 * 1024 * 1024
	defaultChunkSize      = 32 * 1024
	defaultRingBuffer     = 100
	defaultStopTimeout    = 5 * time.Second
)

type Decoder interface {
	Decode(data []byte) (string, error)
}

type DecoderFunc func(data []byte) (string, error)

func (f DecoderFunc) Decode(data []byte) (string, error) {
	return f(data)
}

type Option func(*config)

type config struct {
	dir              string
	env              []string
	cleanEnv         bool
	stdin            io.Reader
	outputLimit      int64
	lineBufferSize   int
	splitMode        SplitMode
	mergeStderr      bool
	decoder          Decoder
	strictDecode     bool
	killProcessGroup bool
	stopTimeout      time.Duration
	waitDelay        time.Duration
	ringBuffer       int
	sink             Sink
	shellName        string
	shellArgs        []string
}

func defaultConfig() config {
	return config{
		outputLimit:    defaultOutputLimit,
		lineBufferSize: defaultLineBufferSize,
		splitMode:      SplitLine,
		decoder: DecoderFunc(func(data []byte) (string, error) {
			return string(data), nil
		}),
		stopTimeout: defaultStopTimeout,
		ringBuffer:  defaultRingBuffer,
	}
}

func applyOptions(opts []Option) config {
	cfg := defaultConfig()
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	if cfg.lineBufferSize <= 0 {
		cfg.lineBufferSize = defaultLineBufferSize
	}
	if cfg.stopTimeout <= 0 {
		cfg.stopTimeout = defaultStopTimeout
	}
	if cfg.ringBuffer < 0 {
		cfg.ringBuffer = 0
	}
	if cfg.decoder == nil {
		cfg.decoder = defaultConfig().decoder
	}
	return cfg
}

func WithDir(dir string) Option {
	return func(c *config) {
		c.dir = dir
	}
}

func WithEnv(env []string) Option {
	return func(c *config) {
		c.env = append(c.env, env...)
	}
}

func WithCleanEnv() Option {
	return func(c *config) {
		c.cleanEnv = true
	}
}

func WithStdin(r io.Reader) Option {
	return func(c *config) {
		c.stdin = r
	}
}

func WithInputString(s string) Option {
	return func(c *config) {
		c.stdin = strings.NewReader(s)
	}
}

func WithOutputLimit(n int64) Option {
	return func(c *config) {
		c.outputLimit = n
	}
}

func WithLineBufferSize(n int) Option {
	return func(c *config) {
		c.lineBufferSize = n
	}
}

func WithSplitMode(mode SplitMode) Option {
	return func(c *config) {
		c.splitMode = mode
	}
}

func WithMergeStderr() Option {
	return func(c *config) {
		c.mergeStderr = true
	}
}

func WithDecoder(decoder Decoder) Option {
	return func(c *config) {
		c.decoder = decoder
	}
}

func WithDecodeFunc(fn func([]byte) (string, error)) Option {
	return func(c *config) {
		if fn == nil {
			c.decoder = nil
			return
		}
		c.decoder = DecoderFunc(fn)
	}
}

func WithStrictDecode() Option {
	return func(c *config) {
		c.strictDecode = true
	}
}

func WithKillProcessGroup() Option {
	return func(c *config) {
		c.killProcessGroup = true
	}
}

func WithStopTimeout(d time.Duration) Option {
	return func(c *config) {
		c.stopTimeout = d
	}
}

func WithWaitDelay(d time.Duration) Option {
	return func(c *config) {
		c.waitDelay = d
	}
}

func WithRingBuffer(n int) Option {
	return func(c *config) {
		c.ringBuffer = n
	}
}

func WithSink(sink Sink) Option {
	return func(c *config) {
		c.sink = sink
	}
}

func WithShell(name string, args ...string) Option {
	return func(c *config) {
		c.shellName = name
		c.shellArgs = append([]string(nil), args...)
	}
}
