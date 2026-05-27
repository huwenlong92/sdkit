package storage

import (
	"context"
	"io"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/huwenlong92/sdkit/pkg/storage/core"
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

type DeleteResult struct {
	Paths   []string
	Deleted bool
	Error   error
}

func (r DeleteResult) OK() bool {
	return r.Error == nil
}

func (r DeleteResult) AfterFailed() bool {
	return r.Deleted && r.Error != nil
}

type ListResult struct {
	Path    string
	Objects []core.Object
	Listed  bool
	Error   error
}

func (r ListResult) OK() bool {
	return r.Error == nil
}

func (r ListResult) AfterFailed() bool {
	return r.Listed && r.Error != nil
}

type SourceResult struct {
	Path   string
	Source string
	Signed bool
	Error  error
}

func (r SourceResult) OK() bool {
	return r.Error == nil
}

func (r SourceResult) AfterFailed() bool {
	return r.Signed && r.Error != nil
}

type TokenResult struct {
	File       core.FileInfo
	Credential *core.UploadCredential
	Issued     bool
	Error      error
}

func (r TokenResult) OK() bool {
	return r.Error == nil
}

func (r TokenResult) AfterFailed() bool {
	return r.Issued && r.Error != nil
}

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
	if err := fs.RegisterHook(HookBeforeUpload, HookValidateFile); err != nil {
		return nil, err
	}
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
	factory := resolveDriver(driver)
	if factory == nil {
		return core.ErrUnknownDriver
	}
	handler, err := factory(fs.cfg)
	if err != nil {
		return err
	}
	fs.handler = handler
	return nil
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

func (fs *FileSystem) CDNURL() string {
	if fs == nil {
		return ""
	}
	return strings.TrimRight(strings.TrimSpace(fs.cfg.Policy.CDNURL), "/")
}

func (fs *FileSystem) AccessPath(objectPath string) string {
	objectPath = strings.TrimSpace(objectPath)
	if objectPath == "" || hasURLScheme(objectPath) {
		return objectPath
	}
	if fs == nil || fs.cfg.DefaultStore || strings.TrimSpace(fs.cfg.StoreName) == "" {
		return objectPath
	}
	if accessURL := core.JoinObjectURL(fs.CDNURL(), objectPath); accessURL != "" {
		return accessURL
	}
	return objectPath
}

func (fs *FileSystem) ObjectPath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || !hasURLScheme(value) {
		return strings.TrimLeft(value, "/")
	}
	baseURL := fs.CDNURL()
	if baseURL == "" {
		return value
	}
	base, err := url.Parse(baseURL)
	if err != nil || base.Scheme == "" || base.Host == "" {
		return value
	}
	u, err := url.Parse(value)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return value
	}
	if !strings.EqualFold(u.Scheme, base.Scheme) || !strings.EqualFold(u.Host, base.Host) {
		return value
	}
	basePath := strings.TrimRight(base.EscapedPath(), "/")
	objectPath := u.EscapedPath()
	if basePath != "" && basePath != "/" {
		if objectPath != basePath && !strings.HasPrefix(objectPath, basePath+"/") {
			return value
		}
		objectPath = strings.TrimPrefix(objectPath, basePath)
	}
	objectPath, err = url.PathUnescape(strings.TrimLeft(objectPath, "/"))
	if err != nil {
		return value
	}
	if objectPath == "" {
		return value
	}
	return objectPath
}

func (fs *FileSystem) Put(file core.FileHeader) UploadResult {
	return fs.PutWithHook(context.Background(), file)
}

func (fs *FileSystem) PutWithHook(ctx context.Context, file core.FileHeader, opts ...HookOption) UploadResult {
	return fs.UploadWithHook(ctx, file, opts...)
}

func (fs *FileSystem) Get(path string) (io.ReadCloser, error) {
	result := fs.GetWithHook(context.Background(), path)
	if result.Error != nil && result.Reader != nil {
		_ = result.Reader.Close()
		return nil, result.Error
	}
	return result.Reader, result.Error
}

func (fs *FileSystem) GetWithHook(ctx context.Context, path string, opts ...HookOption) GetResult {
	return fs.getWithHook(ctx, path, collectHookOptions(opts...))
}

func (fs *FileSystem) get(path string) (io.ReadCloser, error) {
	return fs.handler.Get(path)
}

func (fs *FileSystem) Delete(paths ...string) error {
	result := fs.DeleteWithHook(context.Background(), paths)
	return result.Error
}

func (fs *FileSystem) DeleteWithHook(ctx context.Context, paths []string, opts ...HookOption) DeleteResult {
	return fs.delete(ctx, paths, collectHookOptions(opts...))
}

func (fs *FileSystem) delete(ctx context.Context, paths []string, hooks operationHooks) DeleteResult {
	if ctx == nil {
		ctx = context.Background()
	}
	if fs == nil || fs.handler == nil {
		return DeleteResult{Paths: paths, Error: core.ErrUnknownDriver}
	}
	paths = append([]string(nil), paths...)
	for i, path := range paths {
		paths[i] = fs.ObjectPath(path)
	}
	event := hooks.eventWith(fs, OperationDelete, HookBeforeDelete, core.FileInfo{}, nil, func(event *Event) {
		event.Paths = paths
		if len(paths) == 1 {
			event.Path = paths[0]
			event.File = core.FileInfo{Name: filepath.Base(paths[0]), Path: paths[0]}
		}
	})
	if err := fs.Trigger(ctx, HookBeforeDelete, event, hooks.beforeDelete...); err != nil {
		return DeleteResult{Paths: paths, Error: err}
	}
	if err := fs.handler.Delete(paths...); err != nil {
		_ = fs.Trigger(ctx, HookAfterDeleteFailed, hooks.eventWith(fs, OperationDelete, HookAfterDeleteFailed, event.File, err, func(event *Event) {
			event.Path = firstPath(paths)
			event.Paths = paths
		}), hooks.afterDeleteFailed...)
		return DeleteResult{Paths: paths, Error: err}
	}
	if err := fs.Trigger(ctx, HookAfterDelete, hooks.eventWith(fs, OperationDelete, HookAfterDelete, event.File, nil, func(event *Event) {
		event.Path = firstPath(paths)
		event.Paths = paths
	}), hooks.afterDelete...); err != nil {
		return DeleteResult{Paths: paths, Deleted: true, Error: err}
	}
	return DeleteResult{Paths: paths, Deleted: true}
}

func (fs *FileSystem) List(path string) ([]core.Object, error) {
	result := fs.ListWithHook(context.Background(), path)
	return result.Objects, result.Error
}

func (fs *FileSystem) ListWithHook(ctx context.Context, path string, opts ...HookOption) ListResult {
	return fs.list(ctx, path, collectHookOptions(opts...))
}

func (fs *FileSystem) list(ctx context.Context, path string, hooks operationHooks) ListResult {
	if ctx == nil {
		ctx = context.Background()
	}
	if fs == nil || fs.handler == nil {
		return ListResult{Path: path, Error: core.ErrUnknownDriver}
	}
	event := hooks.eventWith(fs, OperationList, HookBeforeList, core.FileInfo{Name: filepath.Base(path), Path: path}, nil, func(event *Event) {
		event.Path = path
	})
	if err := fs.Trigger(ctx, HookBeforeList, event, hooks.beforeList...); err != nil {
		return ListResult{Path: path, Error: err}
	}
	objects, err := fs.handler.List(path)
	if err != nil {
		_ = fs.Trigger(ctx, HookAfterListFailed, hooks.eventWith(fs, OperationList, HookAfterListFailed, event.File, err, func(event *Event) {
			event.Path = path
		}), hooks.afterListFailed...)
		return ListResult{Path: path, Error: err}
	}
	if err := fs.Trigger(ctx, HookAfterList, hooks.eventWith(fs, OperationList, HookAfterList, event.File, nil, func(event *Event) {
		event.Path = path
		event.Objects = objects
	}), hooks.afterList...); err != nil {
		return ListResult{Path: path, Objects: objects, Listed: true, Error: err}
	}
	return ListResult{Path: path, Objects: objects, Listed: true}
}

func (fs *FileSystem) Source(path string, ttl time.Duration) (string, error) {
	result := fs.SourceWithHook(context.Background(), path, ttl)
	return result.Source, result.Error
}

func (fs *FileSystem) SourceWithHook(ctx context.Context, path string, ttl time.Duration, opts ...HookOption) SourceResult {
	return fs.source(ctx, path, ttl, collectHookOptions(opts...))
}

func (fs *FileSystem) source(ctx context.Context, path string, ttl time.Duration, hooks operationHooks) SourceResult {
	if ctx == nil {
		ctx = context.Background()
	}
	if fs == nil || fs.handler == nil {
		return SourceResult{Path: path, Error: core.ErrUnknownDriver}
	}
	event := hooks.eventWith(fs, OperationSource, HookBeforeSource, core.FileInfo{Name: filepath.Base(path), Path: path}, nil, func(event *Event) {
		event.Path = path
	})
	if err := fs.Trigger(ctx, HookBeforeSource, event, hooks.beforeSource...); err != nil {
		return SourceResult{Path: path, Error: err}
	}
	source, err := fs.handler.Source(path, ttl)
	if err != nil {
		_ = fs.Trigger(ctx, HookAfterSourceFailed, hooks.eventWith(fs, OperationSource, HookAfterSourceFailed, event.File, err, func(event *Event) {
			event.Path = path
		}), hooks.afterSourceFailed...)
		return SourceResult{Path: path, Error: err}
	}
	if err := fs.Trigger(ctx, HookAfterSource, hooks.eventWith(fs, OperationSource, HookAfterSource, event.File, nil, func(event *Event) {
		event.Path = path
		event.Source = source
	}), hooks.afterSource...); err != nil {
		return SourceResult{Path: path, Source: source, Signed: true, Error: err}
	}
	return SourceResult{Path: path, Source: source, Signed: true}
}

func (fs *FileSystem) Token(info core.FileInfo, ttl time.Duration) (*core.UploadCredential, error) {
	result := fs.TokenWithHook(context.Background(), info, ttl)
	return result.Credential, result.Error
}

func (fs *FileSystem) TokenWithHook(ctx context.Context, info core.FileInfo, ttl time.Duration, opts ...HookOption) TokenResult {
	return fs.token(ctx, info, ttl, collectHookOptions(opts...))
}

func (fs *FileSystem) token(ctx context.Context, info core.FileInfo, ttl time.Duration, hooks operationHooks) TokenResult {
	if ctx == nil {
		ctx = context.Background()
	}
	if fs == nil || fs.handler == nil {
		return TokenResult{File: info, Error: core.ErrUnknownDriver}
	}
	info = fs.prepareInfo(info)
	if err := fs.Trigger(ctx, HookBeforeToken, hooks.event(fs, OperationToken, HookBeforeToken, info, nil), hooks.beforeToken...); err != nil {
		return TokenResult{File: info, Error: err}
	}
	credential, err := fs.handler.Token(info, ttl)
	if err != nil {
		_ = fs.Trigger(ctx, HookAfterTokenFailed, hooks.event(fs, OperationToken, HookAfterTokenFailed, info, err), hooks.afterTokenFailed...)
		return TokenResult{File: info, Error: err}
	}
	if err := fs.Trigger(ctx, HookAfterToken, hooks.eventWith(fs, OperationToken, HookAfterToken, info, nil, func(event *Event) {
		event.Credential = credential
	}), hooks.afterToken...); err != nil {
		return TokenResult{File: info, Credential: credential, Issued: true, Error: err}
	}
	return TokenResult{File: info, Credential: credential, Issued: true}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func firstPath(paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	return paths[0]
}

func hasURLScheme(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	return strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://")
}
