package storage

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"

	"github.com/huwenlong92/sdkit/pkg/storage/core"
)

const (
	OperationUpload   = "upload"
	OperationDownload = "download"
	OperationGet      = "get"
	OperationDelete   = "delete"
	OperationList     = "list"
	OperationSource   = "source"
	OperationToken    = "token"

	HookBeforeUpload        = "BeforeUpload"
	HookAfterUpload         = "AfterUpload"
	HookAfterUploadFailed   = "AfterUploadFailed"
	HookBeforeDownload      = "BeforeDownload"
	HookAfterDownload       = "AfterDownload"
	HookAfterDownloadFailed = "AfterDownloadFailed"
	HookBeforeGet           = "BeforeGet"
	HookAfterGet            = "AfterGet"
	HookAfterGetFailed      = "AfterGetFailed"
	HookBeforeDelete        = "BeforeDelete"
	HookAfterDelete         = "AfterDelete"
	HookAfterDeleteFailed   = "AfterDeleteFailed"
	HookBeforeList          = "BeforeList"
	HookAfterList           = "AfterList"
	HookAfterListFailed     = "AfterListFailed"
	HookBeforeSource        = "BeforeSource"
	HookAfterSource         = "AfterSource"
	HookAfterSourceFailed   = "AfterSourceFailed"
	HookBeforeToken         = "BeforeToken"
	HookAfterToken          = "AfterToken"
	HookAfterTokenFailed    = "AfterTokenFailed"
)

var errHookRequired = errors.New("storage hook is nil")

type Event struct {
	Store      string
	Operation  string
	Stage      string
	File       core.FileInfo
	Reader     io.ReadCloser
	Path       string
	Paths      []string
	Objects    []core.Object
	Source     string
	Credential *core.UploadCredential
	Err        error
	Metadata   map[string]any
}

type Hook func(ctx context.Context, event Event) error

type HookOption func(*operationHooks)

type operationHooks struct {
	beforeUpload        []Hook
	afterUpload         []Hook
	afterUploadFailed   []Hook
	beforeDownload      []Hook
	afterDownload       []Hook
	afterDownloadFailed []Hook
	beforeGet           []Hook
	afterGet            []Hook
	afterGetFailed      []Hook
	beforeDelete        []Hook
	afterDelete         []Hook
	afterDeleteFailed   []Hook
	beforeList          []Hook
	afterList           []Hook
	afterListFailed     []Hook
	beforeSource        []Hook
	afterSource         []Hook
	afterSourceFailed   []Hook
	beforeToken         []Hook
	afterToken          []Hook
	afterTokenFailed    []Hook
	metadata            map[string]any
}

func BeforeUpload(hook Hook) HookOption {
	return func(hooks *operationHooks) {
		hooks.beforeUpload = append(hooks.beforeUpload, hook)
	}
}

func AfterUpload(hook Hook) HookOption {
	return func(hooks *operationHooks) {
		hooks.afterUpload = append(hooks.afterUpload, hook)
	}
}

func AfterUploadFailed(hook Hook) HookOption {
	return func(hooks *operationHooks) {
		hooks.afterUploadFailed = append(hooks.afterUploadFailed, hook)
	}
}

func BeforeDownload(hook Hook) HookOption {
	return func(hooks *operationHooks) {
		hooks.beforeDownload = append(hooks.beforeDownload, hook)
	}
}

func AfterDownload(hook Hook) HookOption {
	return func(hooks *operationHooks) {
		hooks.afterDownload = append(hooks.afterDownload, hook)
	}
}

func AfterDownloadFailed(hook Hook) HookOption {
	return func(hooks *operationHooks) {
		hooks.afterDownloadFailed = append(hooks.afterDownloadFailed, hook)
	}
}

func BeforeGet(hook Hook) HookOption {
	return func(hooks *operationHooks) {
		hooks.beforeGet = append(hooks.beforeGet, hook)
	}
}

func AfterGet(hook Hook) HookOption {
	return func(hooks *operationHooks) {
		hooks.afterGet = append(hooks.afterGet, hook)
	}
}

func AfterGetFailed(hook Hook) HookOption {
	return func(hooks *operationHooks) {
		hooks.afterGetFailed = append(hooks.afterGetFailed, hook)
	}
}

func BeforeDelete(hook Hook) HookOption {
	return func(hooks *operationHooks) {
		hooks.beforeDelete = append(hooks.beforeDelete, hook)
	}
}

func AfterDelete(hook Hook) HookOption {
	return func(hooks *operationHooks) {
		hooks.afterDelete = append(hooks.afterDelete, hook)
	}
}

func AfterDeleteFailed(hook Hook) HookOption {
	return func(hooks *operationHooks) {
		hooks.afterDeleteFailed = append(hooks.afterDeleteFailed, hook)
	}
}

func BeforeList(hook Hook) HookOption {
	return func(hooks *operationHooks) {
		hooks.beforeList = append(hooks.beforeList, hook)
	}
}

func AfterList(hook Hook) HookOption {
	return func(hooks *operationHooks) {
		hooks.afterList = append(hooks.afterList, hook)
	}
}

func AfterListFailed(hook Hook) HookOption {
	return func(hooks *operationHooks) {
		hooks.afterListFailed = append(hooks.afterListFailed, hook)
	}
}

func BeforeSource(hook Hook) HookOption {
	return func(hooks *operationHooks) {
		hooks.beforeSource = append(hooks.beforeSource, hook)
	}
}

func AfterSource(hook Hook) HookOption {
	return func(hooks *operationHooks) {
		hooks.afterSource = append(hooks.afterSource, hook)
	}
}

func AfterSourceFailed(hook Hook) HookOption {
	return func(hooks *operationHooks) {
		hooks.afterSourceFailed = append(hooks.afterSourceFailed, hook)
	}
}

func BeforeToken(hook Hook) HookOption {
	return func(hooks *operationHooks) {
		hooks.beforeToken = append(hooks.beforeToken, hook)
	}
}

func AfterToken(hook Hook) HookOption {
	return func(hooks *operationHooks) {
		hooks.afterToken = append(hooks.afterToken, hook)
	}
}

func AfterTokenFailed(hook Hook) HookOption {
	return func(hooks *operationHooks) {
		hooks.afterTokenFailed = append(hooks.afterTokenFailed, hook)
	}
}

func HookMetadata(key string, value any) HookOption {
	return func(hooks *operationHooks) {
		if hooks.metadata == nil {
			hooks.metadata = make(map[string]any)
		}
		hooks.metadata[key] = value
	}
}

func (fs *FileSystem) RegisterHook(stage string, hook Hook) error {
	if hook == nil {
		return errHookRequired
	}
	fs.hookMu.Lock()
	defer fs.hookMu.Unlock()
	fs.hooks[stage] = append(fs.hooks[stage], hook)
	return nil
}

func (fs *FileSystem) CleanHooks(stage string) {
	fs.hookMu.Lock()
	defer fs.hookMu.Unlock()
	if stage == "" {
		fs.hooks = make(map[string][]Hook)
		return
	}
	delete(fs.hooks, stage)
}

func (fs *FileSystem) Trigger(ctx context.Context, stage string, event Event, localHooks ...Hook) error {
	fs.hookMu.RLock()
	hooks := append([]Hook(nil), fs.hooks[stage]...)
	fs.hookMu.RUnlock()
	hooks = append(hooks, localHooks...)
	for _, hook := range hooks {
		if hook == nil {
			continue
		}
		if err := hook(ctx, event); err != nil {
			return err
		}
	}
	return nil
}

func HookValidateFile(ctx context.Context, event Event) error {
	fs, _ := event.Metadata["filesystem"].(*FileSystem)
	if fs == nil {
		return nil
	}
	return fs.ValidateFileInfo(event.File)
}

func collectHookOptions(opts ...HookOption) operationHooks {
	var hooks operationHooks
	for _, opt := range opts {
		if opt != nil {
			opt(&hooks)
		}
	}
	return hooks
}

func (hooks operationHooks) event(fs *FileSystem, operation string, stage string, file core.FileInfo, err error) Event {
	metadata := make(map[string]any, len(hooks.metadata)+1)
	for key, value := range hooks.metadata {
		metadata[key] = value
	}
	metadata["filesystem"] = fs
	return Event{
		Store:     fs.storeName(),
		Operation: operation,
		Stage:     stage,
		File:      file,
		Err:       err,
		Metadata:  metadata,
	}
}

func (hooks operationHooks) eventWith(fs *FileSystem, operation string, stage string, file core.FileInfo, err error, fn func(*Event)) Event {
	event := hooks.event(fs, operation, stage, file, err)
	if fn != nil {
		fn(&event)
	}
	return event
}

func (fs *FileSystem) storeName() string {
	if fs == nil {
		return ""
	}
	if fs.cfg.Policy.Name != "" {
		return fs.cfg.Policy.Name
	}
	return fs.cfg.Driver
}

var reservedNameChars = []string{"\\", "?", "*", "<", "\"", ":", ">", "/", "|"}

func (fs *FileSystem) ValidateFileInfo(file core.FileInfo) error {
	if !fs.ValidateFileSize(file.Size) {
		return core.ErrFileTooBig
	}
	if !fs.ValidateFileName(file.Name, file.Path) {
		return core.ErrNameInvalid
	}
	if !fs.ValidateExtension(file.Name, file.Path) {
		return core.ErrNameInvalid
	}
	return nil
}

func (fs *FileSystem) ValidateFileSize(size int64) bool {
	return fs == nil || fs.cfg.MaxSize <= 0 || size <= fs.cfg.MaxSize
}

func (fs *FileSystem) ValidateFileName(name, path string) bool {
	if name == "" && path != "" {
		name = filepath.Base(path)
	}
	if name == "" || len(name) >= 256 || strings.HasSuffix(name, " ") {
		return false
	}
	for _, ch := range reservedNameChars {
		if strings.Contains(name, ch) {
			return false
		}
	}
	return true
}

func (fs *FileSystem) ValidateExtension(name, path string) bool {
	if fs == nil || len(fs.cfg.AllowedExtensions) == 0 {
		return true
	}
	if name == "" && path != "" {
		name = filepath.Base(path)
	}
	ext := strings.ToLower(filepath.Ext(name))
	for _, allowed := range fs.cfg.AllowedExtensions {
		allowed = strings.ToLower(strings.TrimSpace(allowed))
		if allowed == "" {
			continue
		}
		if !strings.HasPrefix(allowed, ".") {
			allowed = "." + allowed
		}
		if ext == allowed {
			return true
		}
	}
	return false
}
