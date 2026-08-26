// Package baidupan implements RemoteFS by running a separately configured
// BaiduPCS-Go process for each FileSystem instance.
package baidupan

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/huwenlong92/sdkit/pkg/remotefs"
	"github.com/huwenlong92/sdkit/pkg/remotefs/internal/offsetcursor"
)

type Option func(*options)

type options struct {
	runner Runner
}

func WithRunner(runner Runner) Option {
	return func(options *options) {
		options.runner = runner
	}
}

type FileSystem struct {
	runtime       *Runtime
	cfg           SessionConfig
	sessionDir    string
	authMu        sync.Mutex
	stateMu       *sync.Mutex
	authenticated atomic.Bool
	closed        atomic.Bool
}

type removableFileSystem struct {
	*FileSystem
}

type Runtime struct {
	cfg     RuntimeConfig
	runner  Runner
	mu      sync.RWMutex
	version string
}

var sessionStateLocks sync.Map

func NewRuntime(ctx context.Context, cfg RuntimeConfig, opts ...Option) (*Runtime, error) {
	if ctx == nil {
		return nil, remotefs.ErrNilContext
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	normalized, err := cfg.normalized()
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
	if err := validateBinary(normalized.BinaryPath); err != nil {
		return nil, err
	}
	root, err := prepareConfigRoot(normalized.ConfigRoot)
	if err != nil {
		return nil, err
	}
	normalized.ConfigRoot = root
	runtime := &Runtime{cfg: normalized, runner: settings.runner}
	version, err := runtime.checkVersion(ctx)
	if err != nil {
		return nil, err
	}
	runtime.version = version
	return runtime, nil
}

func (r *Runtime) Open(ctx context.Context, cfg SessionConfig) (remotefs.FileSystem, error) {
	if r == nil {
		return nil, invalidConfig("runtime is required")
	}
	if ctx == nil {
		return nil, remotefs.ErrNilContext
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	normalized, err := cfg.normalized()
	if err != nil {
		return nil, err
	}
	sessionDir := filepath.Join(r.cfg.ConfigRoot, accountDirectory(normalized.SessionKey+"\x00"+normalized.CredentialRevision))
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		return nil, &remotefs.Error{Operation: "open", Driver: DriverName, Err: joinStandardError(remotefs.ErrPermissionDenied, err), Summary: "account config directory could not be created"}
	}
	if err := os.Chmod(sessionDir, 0o700); err != nil {
		return nil, &remotefs.Error{Operation: "open", Driver: DriverName, Err: joinStandardError(remotefs.ErrPermissionDenied, err), Summary: "account config directory permissions could not be secured"}
	}
	info, err := os.Lstat(sessionDir)
	if err != nil {
		return nil, &remotefs.Error{Operation: "open", Driver: DriverName, Err: joinStandardError(remotefs.ErrPermissionDenied, err), Summary: "account config directory could not be inspected"}
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, &remotefs.Error{Operation: "open", Driver: DriverName, Err: remotefs.ErrPermissionDenied, Summary: "account config path is not a secure directory"}
	}
	fs := &FileSystem{
		runtime: r, cfg: normalized, sessionDir: sessionDir,
		stateMu: stateLockForSession(sessionDir),
	}
	if err := fs.secureSession(ctx); err != nil {
		return nil, err
	}
	return withOptionalRemove(fs, normalized.AllowRemove), nil
}

func validateBinary(binaryPath string) error {
	info, err := os.Lstat(binaryPath)
	if err != nil {
		return &remotefs.Error{Operation: "version", Driver: DriverName, Err: joinStandardError(ErrBinaryUnavailable, err), Summary: "BaiduPCS-Go binary is unavailable"}
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return &remotefs.Error{Operation: "version", Driver: DriverName, Err: ErrBinaryUnavailable, Summary: "BaiduPCS-Go binary must be an executable regular file"}
	}
	return nil
}

func prepareConfigRoot(configRoot string) (string, error) {
	root, err := filepath.Abs(configRoot)
	if err != nil {
		return "", invalidConfig("config_root cannot be resolved")
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", &remotefs.Error{Operation: "open", Driver: DriverName, Err: joinStandardError(remotefs.ErrPermissionDenied, err), Summary: "config root could not be created"}
	}
	info, err := os.Lstat(root)
	if err != nil {
		return "", &remotefs.Error{Operation: "open", Driver: DriverName, Err: joinStandardError(remotefs.ErrPermissionDenied, err), Summary: "config root could not be inspected"}
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", &remotefs.Error{Operation: "open", Driver: DriverName, Err: remotefs.ErrPermissionDenied, Summary: "config root is not a secure directory"}
	}
	if err := os.Chmod(root, 0o700); err != nil {
		return "", &remotefs.Error{Operation: "open", Driver: DriverName, Err: joinStandardError(remotefs.ErrPermissionDenied, err), Summary: "config root permissions could not be secured"}
	}
	return filepath.Clean(root), nil
}

func stateLockForSession(sessionDir string) *sync.Mutex {
	key := filepath.Clean(sessionDir)
	lock, _ := sessionStateLocks.LoadOrStore(key, &sync.Mutex{})
	return lock.(*sync.Mutex)
}

func (f *FileSystem) Driver() string { return DriverName }

func (f *FileSystem) Stat(ctx context.Context, ref remotefs.Reference) (remotefs.Entry, error) {
	if err := f.ready(ctx); err != nil {
		return remotefs.Entry{}, err
	}
	remotePath, err := f.referencePath(ref)
	if err != nil {
		return remotefs.Entry{}, err
	}
	if err := f.ensureAuthenticated(ctx); err != nil {
		return remotefs.Entry{}, err
	}
	output, err := f.output(ctx, []string{"meta", remotePath})
	if err != nil {
		return remotefs.Entry{}, err
	}
	metas, err := parseMetaOutput(output)
	if err != nil {
		if standard, summary, failed := classifyOutput("stat", output); failed {
			return remotefs.Entry{}, &remotefs.Error{Operation: "stat", Driver: DriverName, Err: standard, Summary: summary}
		}
		return remotefs.Entry{}, &remotefs.Error{Operation: "stat", Driver: DriverName, Err: remotefs.ErrProtocol, Summary: "provider metadata output is invalid"}
	}
	for _, meta := range metas {
		if path.Clean(meta.path) == remotePath {
			entry := f.entry(meta)
			if err := validateSnapshot(ref, "", entry); err != nil {
				return remotefs.Entry{}, err
			}
			return entry, nil
		}
	}
	return remotefs.Entry{}, &remotefs.Error{Operation: "stat", Driver: DriverName, Err: remotefs.ErrTemporary, Summary: "metadata output did not include the requested path"}
}

func (f *FileSystem) List(ctx context.Context, dir remotefs.Reference, opts remotefs.ListOptions) (remotefs.ListPage, error) {
	if err := f.ready(ctx); err != nil {
		return remotefs.ListPage{}, err
	}
	if err := validateListOptions(opts); err != nil {
		return remotefs.ListPage{}, err
	}
	directory, err := f.referencePath(dir)
	if err != nil {
		return remotefs.ListPage{}, err
	}
	if err := f.ensureAuthenticated(ctx); err != nil {
		return remotefs.ListPage{}, err
	}
	args := []string{"ls"}
	switch opts.SortBy {
	case remotefs.SortBySize:
		args = append(args, "--size")
	case remotefs.SortByModTime:
		args = append(args, "--time")
	default:
		args = append(args, "--name")
	}
	if opts.Order == remotefs.SortDescending {
		args = append(args, "--desc")
	} else {
		args = append(args, "--asc")
	}
	args = append(args, directory)
	output, err := f.output(ctx, args)
	if err != nil {
		return remotefs.ListPage{}, err
	}
	listed, err := parseListOutput(directory, output)
	if err != nil {
		if standard, summary, failed := classifyOutput("list", output); failed {
			return remotefs.ListPage{}, &remotefs.Error{Operation: "list", Driver: DriverName, Err: standard, Summary: summary}
		}
		return remotefs.ListPage{}, &remotefs.Error{Operation: "list", Driver: DriverName, Err: remotefs.ErrProtocol, Summary: "provider list output is invalid"}
	}
	cursorScope := listCursorScope(directory, opts)
	start, err := decodeCursor(opts.Cursor, cursorScope, len(listed))
	if err != nil {
		return remotefs.ListPage{}, err
	}
	pageSize := opts.PageSize
	if pageSize <= 0 {
		pageSize = defaultPageSize
	}
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}
	end := start + pageSize
	if end > len(listed) {
		end = len(listed)
	}
	selected := listed[start:end]
	page := remotefs.ListPage{}
	if len(selected) > 0 {
		metaArgs := []string{"meta"}
		for _, item := range selected {
			metaArgs = append(metaArgs, item.path)
		}
		metaOutput, metaErr := f.output(ctx, metaArgs)
		if metaErr != nil {
			return remotefs.ListPage{}, metaErr
		}
		metas, parseErr := parseMetaOutput(metaOutput)
		if parseErr != nil {
			if standard, summary, failed := classifyOutput("list metadata", metaOutput); failed {
				return remotefs.ListPage{}, &remotefs.Error{Operation: "list", Driver: DriverName, Err: standard, Summary: summary}
			}
			return remotefs.ListPage{}, &remotefs.Error{Operation: "list", Driver: DriverName, Err: remotefs.ErrProtocol, Summary: "provider metadata output is invalid"}
		}
		byPath := make(map[string]metaEntry, len(metas))
		for _, meta := range metas {
			byPath[path.Clean(meta.path)] = meta
		}
		for _, item := range selected {
			meta, ok := byPath[path.Clean(item.path)]
			if !ok {
				return remotefs.ListPage{}, &remotefs.Error{Operation: "list", Driver: DriverName, Err: remotefs.ErrTemporary, Summary: "metadata output omitted a listed entry"}
			}
			page.Entries = append(page.Entries, f.entry(meta))
		}
	}
	if end < len(listed) {
		page.NextCursor = offsetcursor.Encode(cursorScope, end)
	}
	return page, nil
}

func (f *FileSystem) Check(ctx context.Context) (remotefs.Health, error) {
	if err := f.ready(ctx); err != nil {
		return remotefs.Health{}, err
	}
	version, err := f.runtime.revalidateVersion(ctx)
	if err != nil {
		return remotefs.Health{Status: remotefs.HealthDegraded}, err
	}
	f.authenticated.Store(false)
	if err := f.ensureAuthenticated(ctx); err != nil {
		return remotefs.Health{Status: remotefs.HealthDegraded, Version: version}, err
	}
	return remotefs.Health{Status: remotefs.HealthHealthy, Version: version}, nil
}

func (f *FileSystem) Quota(ctx context.Context) (remotefs.Quota, error) {
	if err := f.ready(ctx); err != nil {
		return remotefs.Quota{}, err
	}
	if err := f.ensureAuthenticated(ctx); err != nil {
		return remotefs.Quota{}, err
	}
	output, err := f.output(ctx, []string{"quota"})
	if err != nil {
		return remotefs.Quota{}, err
	}
	if standard, summary, failed := classifyOutput("quota", output); failed {
		return remotefs.Quota{}, &remotefs.Error{Operation: "quota", Driver: DriverName, Err: standard, Summary: summary}
	}
	quota, err := parseQuotaOutput(output)
	if err != nil {
		return remotefs.Quota{}, &remotefs.Error{Operation: "quota", Driver: DriverName, Err: remotefs.ErrTemporary, Summary: "provider quota output was not reliable"}
	}
	return quota, nil
}

func (f *FileSystem) remove(ctx context.Context, req remotefs.RemoveRequest) error {
	if err := f.ready(ctx); err != nil {
		return err
	}
	if err := f.ensureAuthenticated(ctx); err != nil {
		return err
	}
	remotePath, err := f.referencePath(req.Reference)
	if err != nil {
		return err
	}
	if remotePath == "/" {
		return &remotefs.Error{Operation: "remove", Driver: DriverName, Err: remotefs.ErrPermissionDenied, Summary: "root cannot be removed"}
	}
	entry, err := f.Stat(ctx, req.Reference)
	if err != nil {
		return err
	}
	if err := validateSnapshot(req.Reference, req.ExpectedVersion, entry); err != nil {
		return err
	}
	output, err := f.outputWithCommand(ctx, Command{
		Name: f.runtime.cfg.BinaryPath, Args: []string{"rm", remotePath}, Env: f.commandEnv(), Input: "y\n", OutputLimit: f.runtime.cfg.OutputLimit, MergeStderr: true,
	})
	if err != nil {
		return err
	}
	if standard, summary, failed := classifyOutput("remove", output); failed {
		return &remotefs.Error{Operation: "remove", Driver: DriverName, Err: standard, Summary: summary}
	}
	if !strings.Contains(output, "操作成功") {
		return errorForOutput("remove", output, f.cfg.BDUSS, f.cfg.STOKEN)
	}
	return nil
}

func (f *removableFileSystem) Remove(ctx context.Context, req remotefs.RemoveRequest) error {
	return f.FileSystem.remove(ctx, req)
}

func (f *FileSystem) Close() error {
	if f != nil {
		f.closed.Store(true)
	}
	return nil
}

func (f *FileSystem) ready(ctx context.Context) error {
	if f == nil || f.closed.Load() {
		return remotefs.ErrClosed
	}
	if ctx == nil {
		return remotefs.ErrNilContext
	}
	return ctx.Err()
}

func (f *FileSystem) ensureAuthenticated(ctx context.Context) error {
	if f.authenticated.Load() {
		return nil
	}
	f.authMu.Lock()
	defer f.authMu.Unlock()
	if f.authenticated.Load() {
		return nil
	}
	unlock, err := f.lockSession(ctx)
	if err != nil {
		return err
	}
	defer unlock()
	if f.authenticated.Load() {
		return nil
	}
	whoOutput, err := f.outputLocked(ctx, []string{"who"})
	if err != nil {
		return err
	}
	if identity, ok := parseWho(whoOutput); ok {
		if err := f.validateIdentity(identity); err != nil {
			return err
		}
		f.authenticated.Store(true)
		return nil
	}
	if f.cfg.BDUSS == "" || f.cfg.STOKEN == "" {
		return &remotefs.Error{Operation: "authenticate", Driver: DriverName, Err: remotefs.ErrUnauthenticated, Summary: "account session is not authenticated and credentials were not provided"}
	}
	loginInput, err := loginScript(f.cfg.BDUSS, f.cfg.STOKEN)
	if err != nil {
		return err
	}
	loginOutput, err := f.outputWithCommandLocked(ctx, Command{
		Name: f.runtime.cfg.BinaryPath, Args: []string{"login"}, Env: f.commandEnv(), Input: loginInput,
		Interactive: true, OutputLimit: f.runtime.cfg.OutputLimit, MergeStderr: true,
	})
	if err != nil {
		return err
	}
	if standard, summary, failed := classifyOutput("authenticate", loginOutput); failed {
		return &remotefs.Error{Operation: "authenticate", Driver: DriverName, Err: standard, Summary: summary}
	}
	whoOutput, err = f.outputLocked(ctx, []string{"who"})
	if err != nil {
		return err
	}
	identity, ok := parseWho(whoOutput)
	if !ok {
		return &remotefs.Error{Operation: "authenticate", Driver: DriverName, Err: remotefs.ErrUnauthenticated, Summary: "account login could not be verified"}
	}
	if err := f.validateIdentity(identity); err != nil {
		return err
	}
	f.authenticated.Store(true)
	return nil
}

func (f *FileSystem) validateIdentity(identity providerIdentity) error {
	if expected := f.cfg.ExpectedProviderUID; expected != "" && identity.UID != expected {
		return &remotefs.Error{Operation: "authenticate", Driver: DriverName, Err: remotefs.ErrIdentityMismatch, Summary: "authenticated provider identity does not match the configured account"}
	}
	return nil
}

func loginScript(bduss string, stoken string) (string, error) {
	if !safeInteractiveValue(bduss) || !safeInteractiveValue(stoken) {
		return "", invalidConfig("credentials contain unsupported characters")
	}
	return "login --bduss=" + bduss + " --stoken=" + stoken + "\nquit\n", nil
}

func safeInteractiveValue(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if character <= ' ' || character == '\x7f' || character == '\\' || character == '"' || character == '\'' {
			return false
		}
	}
	return true
}

func (r *Runtime) checkVersion(ctx context.Context) (string, error) {
	if r == nil || r.runner == nil {
		return "", invalidConfig("runtime is unavailable")
	}
	result, runErr := r.runner.RunOutput(ctx, Command{
		Name: r.cfg.BinaryPath, Args: []string{"--version"}, OutputLimit: r.cfg.OutputLimit, MergeStderr: true,
	})
	output := result.Combined
	if len(output) == 0 {
		output = append(append([]byte(nil), result.Stdout...), result.Stderr...)
	}
	if runErr != nil {
		runErr = safeRunnerError(runErr)
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		if errors.Is(runErr, os.ErrNotExist) || errors.Is(runErr, exec.ErrNotFound) {
			return "", &remotefs.Error{Operation: "version", Driver: DriverName, Err: joinStandardError(ErrBinaryUnavailable, runErr), Summary: "BaiduPCS-Go binary is unavailable"}
		}
		return "", &remotefs.Error{Operation: "version", Driver: DriverName, Err: joinStandardError(remotefs.ErrTemporary, runErr), Summary: "BaiduPCS-Go version check failed"}
	}
	current, err := normalizeVersion(string(output))
	if err != nil {
		return "", &remotefs.Error{Operation: "version", Driver: DriverName, Err: ErrVersionUnsupported, Summary: "BaiduPCS-Go version output is not recognized"}
	}
	if compareVersionStrings(current, r.cfg.MinVersion) < 0 || compareVersionStrings(current, r.cfg.MaxVersion) > 0 {
		return current, &remotefs.Error{Operation: "version", Driver: DriverName, Err: ErrVersionUnsupported, Summary: fmt.Sprintf("BaiduPCS-Go %s is outside the configured version range", current)}
	}
	return current, nil
}

func (r *Runtime) revalidateVersion(ctx context.Context) (string, error) {
	version, err := r.checkVersion(ctx)
	if err != nil {
		return version, err
	}
	r.mu.Lock()
	r.version = version
	r.mu.Unlock()
	return version, nil
}

func (f *FileSystem) output(ctx context.Context, args []string) (string, error) {
	return f.outputWithCommand(ctx, Command{
		Name: f.runtime.cfg.BinaryPath, Args: append([]string(nil), args...), Env: f.commandEnv(), OutputLimit: f.runtime.cfg.OutputLimit, MergeStderr: true,
	})
}

func (f *FileSystem) outputWithCommand(ctx context.Context, command Command) (string, error) {
	unlock, err := f.lockSession(ctx)
	if err != nil {
		return "", err
	}
	defer unlock()
	return f.outputWithCommandLocked(ctx, command)
}

func (f *FileSystem) outputLocked(ctx context.Context, args []string) (string, error) {
	return f.outputWithCommandLocked(ctx, Command{
		Name: f.runtime.cfg.BinaryPath, Args: append([]string(nil), args...), Env: f.commandEnv(), OutputLimit: f.runtime.cfg.OutputLimit, MergeStderr: true,
	})
}

func (f *FileSystem) outputWithCommandLocked(ctx context.Context, command Command) (string, error) {
	result, runErr := f.runtime.runner.RunOutput(ctx, command)
	secureErr := f.secureSessionLocked()
	output := result.Combined
	if len(output) == 0 {
		output = append(append([]byte(nil), result.Stdout...), result.Stderr...)
	}
	if standard, _, classified := classifyOutput("command", string(output)); classified && errors.Is(standard, remotefs.ErrUnauthenticated) {
		f.authenticated.Store(false)
	}
	if runErr != nil {
		runErr = safeRunnerError(runErr)
		if ctx != nil && ctx.Err() != nil {
			return string(output), ctx.Err()
		}
		if standard, summary, classified := classifyOutput("command", string(output)); classified {
			return string(output), &remotefs.Error{Operation: "command", Driver: DriverName, Err: joinStandardError(standard, runErr), Summary: summary}
		}
		return string(output), &remotefs.Error{Operation: "command", Driver: DriverName, Err: joinStandardError(remotefs.ErrTemporary, runErr), Summary: "BaiduPCS-Go command failed"}
	}
	if secureErr != nil {
		return string(output), secureErr
	}
	return string(output), nil
}

func (f *FileSystem) commandEnv() []string {
	return []string{"BAIDUPCS_GO_CONFIG_DIR=" + f.sessionDir}
}

func (f *FileSystem) secureSession(ctx context.Context) error {
	unlock, err := f.lockSession(ctx)
	if err != nil {
		return err
	}
	defer unlock()
	return f.secureSessionLocked()
}

func (f *FileSystem) secureSessionLocked() error {
	err := filepath.WalkDir(f.sessionDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("account config directory contains a symbolic link")
		}
		if entry.IsDir() {
			return os.Chmod(path, 0o700)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("account config directory contains an unsupported file type")
		}
		return os.Chmod(path, 0o600)
	})
	if err != nil {
		return &remotefs.Error{Operation: "secure session", Driver: DriverName, Err: joinStandardError(remotefs.ErrPermissionDenied, err), Summary: "account config permissions could not be secured"}
	}
	return nil
}

func (f *FileSystem) lockSession(ctx context.Context) (func(), error) {
	if ctx == nil {
		return nil, remotefs.ErrNilContext
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f.stateMu.Lock()
	lockPath := filepath.Join(f.sessionDir, ".sdkit-session.lock")
	fd, err := syscall.Open(lockPath, syscall.O_CREAT|syscall.O_RDWR|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		f.stateMu.Unlock()
		return nil, &remotefs.Error{Operation: "lock session", Driver: DriverName, Err: joinStandardError(remotefs.ErrPermissionDenied, err), Summary: "account session lock could not be opened"}
	}
	lockFile := os.NewFile(uintptr(fd), lockPath)
	if lockFile == nil {
		_ = syscall.Close(fd)
		f.stateMu.Unlock()
		return nil, &remotefs.Error{Operation: "lock session", Driver: DriverName, Err: remotefs.ErrTemporary, Summary: "account session lock could not be created"}
	}
	info, err := lockFile.Stat()
	if err != nil || !info.Mode().IsRegular() {
		_ = lockFile.Close()
		f.stateMu.Unlock()
		return nil, &remotefs.Error{Operation: "lock session", Driver: DriverName, Err: joinStandardError(remotefs.ErrPermissionDenied, err), Summary: "account session lock is not a regular file"}
	}
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		err = syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			break
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) {
			_ = lockFile.Close()
			f.stateMu.Unlock()
			return nil, &remotefs.Error{Operation: "lock session", Driver: DriverName, Err: joinStandardError(remotefs.ErrTemporary, err), Summary: "account session lock could not be acquired"}
		}
		select {
		case <-ctx.Done():
			_ = lockFile.Close()
			f.stateMu.Unlock()
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			_ = syscall.Flock(fd, syscall.LOCK_UN)
			_ = lockFile.Close()
			f.stateMu.Unlock()
		})
	}, nil
}

func (f *FileSystem) referencePath(ref remotefs.Reference) (string, error) {
	return normalizeRemotePath(ref.Path, true)
}

func validateListOptions(opts remotefs.ListOptions) error {
	if opts.PageSize < 0 || opts.PageSize > maxPageSize {
		return &remotefs.Error{Operation: "list", Driver: DriverName, Err: remotefs.ErrInvalidOption, Summary: "page size is outside the supported range"}
	}
	switch opts.SortBy {
	case "", remotefs.SortByName, remotefs.SortBySize, remotefs.SortByModTime:
	default:
		return &remotefs.Error{Operation: "list", Driver: DriverName, Err: remotefs.ErrInvalidOption, Summary: "sort field is invalid"}
	}
	switch opts.Order {
	case "", remotefs.SortAscending, remotefs.SortDescending:
	default:
		return &remotefs.Error{Operation: "list", Driver: DriverName, Err: remotefs.ErrInvalidOption, Summary: "sort order is invalid"}
	}
	return nil
}

func normalizeRemotePath(value string, requireExplicitAbsolute bool) (string, error) {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	if requireExplicitAbsolute && !strings.HasPrefix(value, "/") {
		return "", &remotefs.Error{Operation: "resolve", Driver: DriverName, Err: remotefs.ErrInvalidReference, Summary: "remote path must be absolute"}
	}
	if value == "" || value == "." {
		value = "/"
	}
	if strings.ContainsRune(value, '\x00') {
		return "", &remotefs.Error{Operation: "resolve", Driver: DriverName, Err: remotefs.ErrInvalidReference, Summary: "remote path contains an invalid character"}
	}
	for _, segment := range strings.Split(value, "/") {
		if segment == ".." {
			return "", &remotefs.Error{Operation: "resolve", Driver: DriverName, Err: remotefs.ErrPermissionDenied, Summary: "remote parent traversal is not allowed"}
		}
	}
	return path.Clean("/" + strings.TrimPrefix(value, "/")), nil
}

func (f *FileSystem) entry(meta metaEntry) remotefs.Entry {
	entryType := remotefs.EntryFile
	if meta.isDir {
		entryType = remotefs.EntryDirectory
	}
	ref := remotefs.Reference{ID: meta.fsID, Path: path.Clean(meta.path)}
	modifiedAt := meta.modifiedAt
	entry := remotefs.Entry{
		Reference: ref,
		Name:      meta.name,
		Type:      entryType,
		Size:      meta.size,
		ModTime:   &modifiedAt,
		Version:   meta.appID,
	}
	if meta.md5 != "" {
		entry.Checksum = &remotefs.Checksum{Algorithm: "md5", Value: meta.md5}
	}
	return entry
}

func validateSnapshot(ref remotefs.Reference, expectedVersion string, entry remotefs.Entry) error {
	if ref.ID != "" && ref.ID != entry.Reference.ID || expectedVersion != "" && expectedVersion != entry.Version {
		return &remotefs.Error{Operation: "validate", Driver: DriverName, Err: remotefs.ErrSourceChanged, Summary: "source snapshot no longer matches the request"}
	}
	return nil
}

func listCursorScope(remotePath string, opts remotefs.ListOptions) string {
	sortBy := opts.SortBy
	if sortBy == "" {
		sortBy = remotefs.SortByName
	}
	order := opts.Order
	if order == "" {
		order = remotefs.SortAscending
	}
	return offsetcursor.Scope(remotePath, string(sortBy), string(order))
}

func decodeCursor(cursor string, scope string, length int) (int, error) {
	value, err := offsetcursor.Decode(cursor, scope, length)
	if err != nil {
		return 0, &remotefs.Error{Operation: "list", Driver: DriverName, Err: remotefs.ErrInvalidOption, Summary: "pagination cursor does not belong to this query"}
	}
	return value, nil
}

func accountDirectory(account string) string {
	hash := sha256.Sum256([]byte(account))
	return hex.EncodeToString(hash[:])
}

func withOptionalRemove(fs *FileSystem, enabled bool) remotefs.FileSystem {
	if enabled {
		return &removableFileSystem{FileSystem: fs}
	}
	return fs
}
