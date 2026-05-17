package filesystem

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/huwenlong92/sdkit/pkg/filesystem/core"
	"github.com/huwenlong92/sdkit/pkg/filesystem/driver/cos"
	"github.com/huwenlong92/sdkit/pkg/filesystem/driver/local"
	"github.com/huwenlong92/sdkit/pkg/filesystem/driver/oss"
	"github.com/huwenlong92/sdkit/pkg/filesystem/driver/s3"
)

type FileSystem struct {
	handler core.Handler
	cfg     core.Config
	namer   *core.Namer

	hookMu sync.RWMutex
	hooks  map[string][]Hook

	sessionMu *sync.RWMutex
	sessions  map[string]*UploadSession
}

type Option func(*core.Config)

func New(cfg *core.Config) (*FileSystem, error) {
	if cfg == nil {
		cfg = &core.Config{}
	}
	normalized := normalizeConfig(*cfg)
	fs := &FileSystem{
		cfg:       normalized,
		namer:     &core.Namer{DirRule: normalized.DirRule, FileRule: normalized.FileRule},
		hooks:     make(map[string][]Hook),
		sessionMu: &sync.RWMutex{},
		sessions:  make(map[string]*UploadSession),
	}
	if err := fs.DispatchHandler(); err != nil {
		return nil, err
	}
	fs.Use(HookBeforeUpload, HookValidateFile)
	return fs, nil
}

func (fs *FileSystem) Close() error {
	if fs == nil {
		return nil
	}
	if closer, ok := fs.handler.(interface{ Close() error }); ok {
		if err := closer.Close(); err != nil {
			return err
		}
	}
	fs.handler = nil
	fs.namer = nil
	fs.hooks = nil
	fs.sessions = nil
	fs.sessionMu = nil
	return nil
}

func (fs *FileSystem) Recycle() {
	_ = fs.Close()
}

func (fs *FileSystem) DispatchHandler() error {
	if fs == nil {
		return core.ErrUnknownDriver
	}
	policy := fs.cfg.Policy
	driver := firstNonEmpty(policy.Driver, fs.cfg.Driver)
	switch driver {
	case "local":
		fs.handler = local.NewFromConfig(fs.cfg)
		return nil
	case "s3":
		handler, err := s3.NewFromConfig(fs.cfg, false)
		fs.handler = handler
		return err
	case "minio":
		handler, err := s3.NewFromConfig(fs.cfg, true)
		fs.handler = handler
		return err
	case "cos":
		handler, err := cos.NewFromConfig(fs.cfg)
		fs.handler = handler
		return err
	case "oss":
		handler, err := oss.NewFromConfig(fs.cfg)
		fs.handler = handler
		return err
	default:
		return core.ErrUnknownDriver
	}
}

func NewFromPolicy(policy core.StoragePolicy, opts ...Option) (*FileSystem, error) {
	cfg := &core.Config{
		Driver: policy.Driver,
		Policy: policy,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(cfg)
		}
	}
	return New(cfg)
}

func NewFileSystem(policy core.StoragePolicy, opts ...Option) (*FileSystem, error) {
	return NewFromPolicy(policy, opts...)
}

func WithUploadDir(uploadDir string) Option {
	return func(cfg *core.Config) {
		cfg.UploadDir = uploadDir
	}
}

func WithTempDir(tempDir string) Option {
	return func(cfg *core.Config) {
		cfg.TempDir = tempDir
	}
}

func WithMaxSize(maxSize int64) Option {
	return func(cfg *core.Config) {
		cfg.MaxSize = maxSize
	}
}

func WithChunkSize(chunkSize int64) Option {
	return func(cfg *core.Config) {
		cfg.ChunkSize = chunkSize
	}
}

func WithTokenTTL(ttl time.Duration) Option {
	return func(cfg *core.Config) {
		cfg.TokenTTL = ttl
	}
}

func WithNameRules(dirRule, fileRule string) Option {
	return func(cfg *core.Config) {
		cfg.DirRule = dirRule
		cfg.FileRule = fileRule
	}
}

func WithAllowedExtensions(exts ...string) Option {
	return func(cfg *core.Config) {
		cfg.AllowedExtensions = append([]string(nil), exts...)
	}
}

func (fs *FileSystem) Handler() core.Handler {
	if fs == nil {
		return nil
	}
	return fs.handler
}

func (fs *FileSystem) Config() core.Config {
	if fs == nil {
		return core.Config{}
	}
	return fs.cfg
}

func (fs *FileSystem) Put(file core.FileHeader) error {
	return fs.Upload(context.Background(), file)
}

func (fs *FileSystem) Get(path string) (io.ReadCloser, error) {
	return fs.handler.Get(path)
}

func (fs *FileSystem) Delete(paths ...string) error {
	return fs.handler.Delete(paths...)
}

func (fs *FileSystem) List(path string) ([]core.Object, error) {
	return fs.handler.List(path)
}

func (fs *FileSystem) Source(path string, ttl time.Duration) (string, error) {
	return fs.handler.Source(path, ttl)
}

func (fs *FileSystem) Token(info core.FileInfo, ttl time.Duration) (*core.UploadCredential, error) {
	return fs.handler.Token(fs.prepareInfo(info), ttl)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
