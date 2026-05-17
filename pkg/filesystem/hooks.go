package filesystem

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	"github.com/huwenlong92/sdkit/pkg/filesystem/core"
)

const (
	HookBeforeUpload        = "BeforeUpload"
	HookAfterUpload         = "AfterUpload"
	HookAfterUploadFailed   = "AfterUploadFailed"
	HookAfterValidateFailed = "AfterValidateFailed"
)

var errHookRequired = errors.New("filesystem hook is nil")

type Hook func(ctx context.Context, fs *FileSystem, file core.FileInfo) error

var reservedNameChars = []string{"\\", "?", "*", "<", "\"", ":", ">", "/", "|"}

func (fs *FileSystem) Use(name string, hook Hook) error {
	if hook == nil {
		return errHookRequired
	}
	fs.hookMu.Lock()
	defer fs.hookMu.Unlock()
	fs.hooks[name] = append(fs.hooks[name], hook)
	return nil
}

func (fs *FileSystem) CleanHooks(name string) {
	fs.hookMu.Lock()
	defer fs.hookMu.Unlock()
	if name == "" {
		fs.hooks = make(map[string][]Hook)
		return
	}
	delete(fs.hooks, name)
}

func (fs *FileSystem) Trigger(ctx context.Context, name string, file core.FileInfo) error {
	fs.hookMu.RLock()
	hooks := append([]Hook(nil), fs.hooks[name]...)
	fs.hookMu.RUnlock()
	for _, hook := range hooks {
		if err := hook(ctx, fs, file); err != nil {
			return err
		}
	}
	return nil
}

func HookValidateFile(ctx context.Context, fs *FileSystem, file core.FileInfo) error {
	if fs == nil {
		return nil
	}
	return fs.ValidateFileInfo(file)
}

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
