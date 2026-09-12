package baidupan_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/huwenlong92/sdkit/pkg/execx"
	"github.com/huwenlong92/sdkit/pkg/remotefs"
	"github.com/huwenlong92/sdkit/pkg/remotefs/driver/baidupan"
	"github.com/huwenlong92/sdkit/tests/pkg/remotefs/testsuite"
)

const (
	testBDUSS  = "fixture-bduss-secret"
	testSTOKEN = "fixture-stoken-secret"
)

type fakeBaiduRunner struct {
	mu        sync.Mutex
	commands  []baidupan.Command
	outputFn  func(context.Context, baidupan.Command) ([]byte, error)
	streamFn  func(context.Context, baidupan.Command, execx.Sink) (execx.Result, error)
	active    atomic.Int32
	maxActive atomic.Int32
}

func (r *fakeBaiduRunner) RunOutput(ctx context.Context, command baidupan.Command) (execx.OutputResult, error) {
	r.record(command)
	r.enter()
	defer r.leave()
	output, err := r.output(ctx, command)
	return execx.OutputResult{
		Result:   execx.Result{Command: command.Name, Args: append([]string(nil), command.Args...), ExitCode: exitCode(err)},
		Stdout:   append([]byte(nil), output...),
		Combined: append([]byte(nil), output...),
	}, err
}

func (r *fakeBaiduRunner) RunStream(ctx context.Context, command baidupan.Command, sink execx.Sink) (execx.Result, error) {
	r.record(command)
	r.enter()
	defer r.leave()
	if r.streamFn == nil {
		return execx.Result{Command: command.Name, Args: append([]string(nil), command.Args...), ExitCode: 0}, nil
	}
	return r.streamFn(ctx, command, sink)
}

func (r *fakeBaiduRunner) output(ctx context.Context, command baidupan.Command) ([]byte, error) {
	if r.outputFn != nil {
		if output, err := r.outputFn(ctx, command); output != nil || err != nil {
			return output, err
		}
	}
	switch commandName(command) {
	case "--version":
		return []byte("BaiduPCS-Go version v4.0.0\n"), nil
	case "who":
		return []byte("当前帐号 uid: 10001, 用户名: fixture-user, 性别: , 年龄: 0.0\n"), nil
	case "ls":
		return []byte(loadFixturePath("testdata/ls_success.txt")), nil
	case "meta":
		return []byte(loadFixturePath("testdata/meta_success.txt")), nil
	case "quota":
		return []byte("用户名: fixture-user, 总空间: 2TB, 已用空间: 512GB, 比率: 25%\n"), nil
	case "cd":
		return []byte("改变工作目录成功\n"), nil
	case "mkdir":
		return []byte("创建目录成功\n"), nil
	case "transfer":
		return []byte(loadFixturePath("testdata/transfer_success.txt")), nil
	case "rm":
		return []byte("操作成功, 文件已移至回收站\n"), nil
	default:
		return nil, fmt.Errorf("unexpected command: %v", command.Args)
	}
}

func (r *fakeBaiduRunner) record(command baidupan.Command) {
	command.Args = append([]string(nil), command.Args...)
	command.Env = append([]string(nil), command.Env...)
	r.mu.Lock()
	r.commands = append(r.commands, command)
	r.mu.Unlock()
}

func (r *fakeBaiduRunner) snapshot() []baidupan.Command {
	r.mu.Lock()
	defer r.mu.Unlock()
	commands := make([]baidupan.Command, len(r.commands))
	copy(commands, r.commands)
	return commands
}

func (r *fakeBaiduRunner) enter() {
	active := r.active.Add(1)
	for {
		current := r.maxActive.Load()
		if active <= current || r.maxActive.CompareAndSwap(current, active) {
			return
		}
	}
}

func (r *fakeBaiduRunner) leave() { r.active.Add(-1) }

func exitCode(err error) int {
	if err != nil {
		return 1
	}
	return 0
}

func commandName(command baidupan.Command) string {
	if len(command.Args) == 0 {
		return ""
	}
	return command.Args[0]
}

func loadFixturePath(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	return string(data)
}

type baiduTestConfig struct {
	runtime baidupan.RuntimeConfig
	session baidupan.SessionConfig
}

func newTestBinary(t *testing.T) string {
	t.Helper()
	binaryPath := filepath.Join(t.TempDir(), "BaiduPCS-Go")
	if err := os.WriteFile(binaryPath, []byte("test binary"), 0o700); err != nil {
		t.Fatalf("write test binary: %v", err)
	}
	return binaryPath
}

func newBaiduFileSystem(t *testing.T, runner baidupan.Runner, account string, options ...func(*baiduTestConfig)) remotefs.FileSystem {
	t.Helper()
	cfg := baiduTestConfig{
		runtime: baidupan.RuntimeConfig{
			BinaryPath: newTestBinary(t), MinVersion: "v4.0.0", MaxVersion: "v4.0.0",
			ConfigRoot: t.TempDir(), OutputLimit: 1024 * 1024,
		},
		session: baidupan.SessionConfig{
			SessionKey: account, BDUSS: testBDUSS, STOKEN: testSTOKEN,
			DownloadConcurrency: 2, DownloadMode: "locate",
		},
	}
	for _, option := range options {
		option(&cfg)
	}
	runtime, err := baidupan.NewRuntime(context.Background(), cfg.runtime, baidupan.WithRunner(runner))
	if err != nil {
		t.Fatalf("new baidupan runtime: %v", err)
	}
	fs, err := runtime.Open(context.Background(), cfg.session)
	if err != nil {
		t.Fatalf("new baidupan: %v", err)
	}
	t.Cleanup(func() { _ = fs.Close() })
	return fs
}

func baiduStager(t *testing.T, fs remotefs.FileSystem) remotefs.ShareStager {
	t.Helper()
	stager, ok := fs.(remotefs.ShareStager)
	if !ok {
		t.Fatal("baidupan driver does not implement ShareStager")
	}
	return stager
}

func TestBaiduPanNewChecksVersionAndSecuresHashedSessionDirectory(t *testing.T) {
	runner := &fakeBaiduRunner{outputFn: func(_ context.Context, command baidupan.Command) ([]byte, error) {
		if commandName(command) != "who" {
			return nil, nil
		}
		configDir := envValue(command.Env, "BAIDUPCS_GO_CONFIG_DIR")
		if err := os.WriteFile(filepath.Join(configDir, "pcs_config.json"), []byte("{}"), 0o644); err != nil {
			return nil, err
		}
		return nil, nil
	}}
	account := "../../sensitive-account-name"
	fs := newBaiduFileSystem(t, runner, account)
	if fs.Driver() != baidupan.DriverName {
		t.Fatalf("driver = %q", fs.Driver())
	}
	if _, ok := fs.(remotefs.Remover); ok {
		t.Fatal("baidupan unexpectedly reports Remove when disabled")
	}
	if _, err := fs.Stat(context.Background(), remotefs.Reference{Path: "/shows/episode 01.mp4"}); err != nil {
		t.Fatalf("stat: %v", err)
	}
	commands := runner.snapshot()
	if len(commands) < 3 || commandName(commands[0]) != "--version" {
		t.Fatalf("commands = %+v", commands)
	}
	configDir := ""
	for _, command := range commands {
		if commandName(command) == "who" {
			configDir = envValue(command.Env, "BAIDUPCS_GO_CONFIG_DIR")
			break
		}
	}
	if configDir == "" || strings.Contains(configDir, "sensitive-account-name") || !filepath.IsAbs(configDir) {
		t.Fatalf("unsafe config dir %q", configDir)
	}
	info, err := os.Stat(configDir)
	if err != nil {
		t.Fatalf("stat config dir: %v", err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("config dir mode = %o, want 700", info.Mode().Perm())
	}
	configInfo, err := os.Stat(filepath.Join(configDir, "pcs_config.json"))
	if err != nil {
		t.Fatalf("stat config file: %v", err)
	}
	if configInfo.Mode().Perm() != 0o600 {
		t.Fatalf("config file mode = %o, want 600", configInfo.Mode().Perm())
	}
}

func TestBaiduPanNewReportsBinaryAndVersionErrors(t *testing.T) {
	tests := []struct {
		name   string
		runner *fakeBaiduRunner
		want   error
	}{
		{
			name: "binary missing",
			runner: &fakeBaiduRunner{outputFn: func(context.Context, baidupan.Command) ([]byte, error) {
				return nil, exec.ErrNotFound
			}},
			want: baidupan.ErrBinaryUnavailable,
		},
		{
			name: "version unsupported",
			runner: &fakeBaiduRunner{outputFn: func(context.Context, baidupan.Command) ([]byte, error) {
				return []byte("BaiduPCS-Go version v3.9.9\n"), nil
			}},
			want: baidupan.ErrVersionUnsupported,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			binaryPath := newTestBinary(t)
			if errors.Is(test.want, baidupan.ErrBinaryUnavailable) {
				binaryPath = filepath.Join(t.TempDir(), "missing-BaiduPCS-Go")
			}
			_, err := baidupan.NewRuntime(context.Background(), baidupan.RuntimeConfig{
				BinaryPath: binaryPath, MinVersion: "v4.0.0", MaxVersion: "v4.0.0",
				ConfigRoot: t.TempDir(),
			}, baidupan.WithRunner(test.runner))
			if !errors.Is(err, test.want) {
				t.Fatalf("New() error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestBaiduPanRuntimeChecksVersionOnceAndOpensTypedSessions(t *testing.T) {
	var versionCalls atomic.Int32
	runner := &fakeBaiduRunner{outputFn: func(_ context.Context, command baidupan.Command) ([]byte, error) {
		if commandName(command) == "--version" {
			versionCalls.Add(1)
		}
		return nil, nil
	}}
	runtime, err := baidupan.NewRuntime(context.Background(), baidupan.RuntimeConfig{
		BinaryPath: newTestBinary(t), MinVersion: "v4.0.0", MaxVersion: "v4.0.0",
		ConfigRoot: t.TempDir(),
	}, baidupan.WithRunner(runner))
	if err != nil {
		t.Fatal(err)
	}
	for _, sessionKey := range []string{"account-one", "account-two"} {
		fs, openErr := runtime.Open(context.Background(), baidupan.SessionConfig{SessionKey: sessionKey})
		if openErr != nil {
			t.Fatalf("open %s: %v", sessionKey, openErr)
		}
		_ = fs.Close()
	}
	if versionCalls.Load() != 1 {
		t.Fatalf("version calls = %d, want 1", versionCalls.Load())
	}
}

func TestBaiduPanRuntimeAndSessionRejectUnsafeBounds(t *testing.T) {
	_, err := baidupan.NewRuntime(context.Background(), baidupan.RuntimeConfig{
		BinaryPath: newTestBinary(t), MinVersion: "v4.0.0", MaxVersion: "v4.0.0",
		ConfigRoot: t.TempDir(), OutputLimit: 65 * 1024 * 1024,
	}, baidupan.WithRunner(&fakeBaiduRunner{}))
	if !errors.Is(err, remotefs.ErrInvalidArgument) {
		t.Fatalf("oversized output limit error = %v", err)
	}
	runtime, err := baidupan.NewRuntime(context.Background(), baidupan.RuntimeConfig{
		BinaryPath: newTestBinary(t), MinVersion: "v4.0.0", MaxVersion: "v4.0.0", ConfigRoot: t.TempDir(),
	}, baidupan.WithRunner(&fakeBaiduRunner{}))
	if err != nil {
		t.Fatal(err)
	}
	_, err = runtime.Open(context.Background(), baidupan.SessionConfig{SessionKey: "account", DownloadConcurrency: 33})
	if !errors.Is(err, remotefs.ErrInvalidArgument) {
		t.Fatalf("oversized download concurrency error = %v", err)
	}
	nonExecutable := filepath.Join(t.TempDir(), "BaiduPCS-Go")
	if err := os.WriteFile(nonExecutable, []byte("binary"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = baidupan.NewRuntime(context.Background(), baidupan.RuntimeConfig{
		BinaryPath: nonExecutable, MinVersion: "v4.0.0", MaxVersion: "v4.0.0", ConfigRoot: t.TempDir(),
	}, baidupan.WithRunner(&fakeBaiduRunner{}))
	if !errors.Is(err, baidupan.ErrBinaryUnavailable) {
		t.Fatalf("non-executable binary error = %v", err)
	}
	symlinkPath := filepath.Join(t.TempDir(), "BaiduPCS-Go")
	if err := os.Symlink(newTestBinary(t), symlinkPath); err != nil {
		t.Fatal(err)
	}
	_, err = baidupan.NewRuntime(context.Background(), baidupan.RuntimeConfig{
		BinaryPath: symlinkPath, MinVersion: "v4.0.0", MaxVersion: "v4.0.0", ConfigRoot: t.TempDir(),
	}, baidupan.WithRunner(&fakeBaiduRunner{}))
	if !errors.Is(err, baidupan.ErrBinaryUnavailable) {
		t.Fatalf("symlink binary error = %v", err)
	}
}

func TestBaiduPanExecRunnerDoesNotInheritParentSecrets(t *testing.T) {
	t.Setenv("SDKIT_PARENT_SECRET", "must-not-reach-provider")
	binaryPath := filepath.Join(t.TempDir(), "BaiduPCS-Go")
	script := `#!/bin/sh
if [ -n "$SDKIT_PARENT_SECRET" ]; then
  echo "parent secret leaked"
  exit 1
fi
echo "BaiduPCS-Go version v4.0.0"
`
	if err := os.WriteFile(binaryPath, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	probe, probeErr := (baidupan.ExecRunner{}).RunOutput(context.Background(), baidupan.Command{
		Name: binaryPath, Args: []string{"--version"}, OutputLimit: 1024 * 1024, MergeStderr: true,
	})
	if probeErr != nil {
		t.Fatalf("clean environment probe: %v: %s", probeErr, probe.Combined)
	}
	if _, err := baidupan.NewRuntime(context.Background(), baidupan.RuntimeConfig{
		BinaryPath: binaryPath, MinVersion: "v4.0.0", MaxVersion: "v4.0.0", ConfigRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("new runtime with clean environment: %v", err)
	}
}

func TestBaiduPanRejectsSymlinkInAccountConfigDirectory(t *testing.T) {
	outside := filepath.Join(t.TempDir(), "outside.json")
	if err := os.WriteFile(outside, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write outside file: %v", err)
	}
	runner := &fakeBaiduRunner{outputFn: func(_ context.Context, command baidupan.Command) ([]byte, error) {
		if commandName(command) == "who" {
			configDir := envValue(command.Env, "BAIDUPCS_GO_CONFIG_DIR")
			if err := os.Symlink(outside, filepath.Join(configDir, "unexpected-link")); err != nil {
				return nil, err
			}
			return []byte("当前帐号 uid: 10001, 用户名: fixture-user, 性别: , 年龄: 0.0\n"), nil
		}
		return nil, nil
	}}
	runtime, err := baidupan.NewRuntime(context.Background(), baidupan.RuntimeConfig{
		BinaryPath: newTestBinary(t), MinVersion: "v4.0.0", MaxVersion: "v4.0.0",
		ConfigRoot: t.TempDir(),
	}, baidupan.WithRunner(runner))
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}
	fs, err := runtime.Open(context.Background(), baidupan.SessionConfig{SessionKey: "account"})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	_, err = fs.Stat(context.Background(), remotefs.Reference{Path: "/shows/episode 01.mp4"})
	if !errors.Is(err, remotefs.ErrPermissionDenied) {
		t.Fatalf("Stat() error = %v, want ErrPermissionDenied", err)
	}
}

func TestBaiduPanStatListAndTypedErrors(t *testing.T) {
	runner := &fakeBaiduRunner{}
	fs := newBaiduFileSystem(t, runner, "account")
	entry, err := fs.Stat(context.Background(), remotefs.Reference{Path: "/shows/episode 01.mp4"})
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if entry.Name != "episode 01.mp4" || entry.Size != 10 || entry.Reference.ID != "101" || entry.Checksum == nil {
		t.Fatalf("entry = %+v", entry)
	}
	first, err := fs.List(context.Background(), remotefs.Reference{Path: "/shows"}, remotefs.ListOptions{PageSize: 1})
	if err != nil {
		t.Fatalf("list first: %v", err)
	}
	if len(first.Entries) != 1 || first.Entries[0].Reference.Path != "/shows/episode 01.mp4" || first.NextCursor == "" {
		t.Fatalf("first page = %+v", first)
	}
	second, err := fs.List(context.Background(), remotefs.Reference{Path: "/shows"}, remotefs.ListOptions{PageSize: 1, Cursor: first.NextCursor})
	if err != nil {
		t.Fatalf("list second: %v", err)
	}
	if len(second.Entries) != 1 || second.Entries[0].Type != remotefs.EntryDirectory || second.NextCursor != "" {
		t.Fatalf("second page = %+v", second)
	}
	for _, request := range []struct {
		directory remotefs.Reference
		options   remotefs.ListOptions
	}{
		{directory: remotefs.Reference{Path: "/other"}, options: remotefs.ListOptions{PageSize: 1, Cursor: first.NextCursor}},
		{directory: remotefs.Reference{Path: "/shows"}, options: remotefs.ListOptions{PageSize: 1, Cursor: first.NextCursor, SortBy: remotefs.SortBySize}},
	} {
		if _, err := fs.List(context.Background(), request.directory, request.options); !errors.Is(err, remotefs.ErrInvalidOption) {
			t.Fatalf("cross-query cursor error = %v", err)
		}
	}

	checks := []struct {
		fixture string
		want    error
	}{
		{"not_found.txt", remotefs.ErrNotFound},
		{"unauthenticated.txt", remotefs.ErrUnauthenticated},
		{"rate_limited.txt", remotefs.ErrRateLimited},
		{"unknown.txt", remotefs.ErrProtocol},
	}
	for _, check := range checks {
		t.Run(check.fixture, func(t *testing.T) {
			errorRunner := &fakeBaiduRunner{outputFn: func(_ context.Context, command baidupan.Command) ([]byte, error) {
				if commandName(command) == "meta" {
					return []byte(loadFixturePath("testdata/" + check.fixture)), nil
				}
				return nil, nil
			}}
			errorFS := newBaiduFileSystem(t, errorRunner, "error-account")
			_, err := errorFS.Stat(context.Background(), remotefs.Reference{Path: "/missing"})
			if !errors.Is(err, check.want) {
				t.Fatalf("Stat() error = %v, want %v", err, check.want)
			}
		})
	}
}

func TestBaiduPanErrorsRedactCredentials(t *testing.T) {
	runner := &fakeBaiduRunner{outputFn: func(_ context.Context, command baidupan.Command) ([]byte, error) {
		if commandName(command) == "meta" {
			return []byte("操作失败: " + testBDUSS + " " + testSTOKEN), nil
		}
		return nil, nil
	}}
	fs := newBaiduFileSystem(t, runner, "account")
	_, err := fs.Stat(context.Background(), remotefs.Reference{Path: "/secret"})
	if err == nil {
		t.Fatal("Stat() error = nil")
	}
	if strings.Contains(err.Error(), testBDUSS) || strings.Contains(err.Error(), testSTOKEN) {
		t.Fatalf("error leaked credentials: %v", err)
	}
}

func TestBaiduPanErrorsDoNotExposeExecArguments(t *testing.T) {
	runner := &fakeBaiduRunner{outputFn: func(_ context.Context, command baidupan.Command) ([]byte, error) {
		if commandName(command) == "meta" {
			return nil, &execx.ExitError{
				Result: execx.Result{Command: command.Name, Args: []string{"--bduss=" + testBDUSS, "--stoken=" + testSTOKEN}, ExitCode: 1},
				Err:    errors.New("exit status 1"),
			}
		}
		return nil, nil
	}}
	fs := newBaiduFileSystem(t, runner, "account")
	_, err := fs.Stat(context.Background(), remotefs.Reference{Path: "/secret"})
	if err == nil {
		t.Fatal("Stat() error = nil")
	}
	var exitErr *execx.ExitError
	if errors.As(err, &exitErr) {
		t.Fatalf("error exposes execx result arguments: %+v", exitErr.Result.Args)
	}
	if strings.Contains(err.Error(), testBDUSS) || strings.Contains(err.Error(), testSTOKEN) {
		t.Fatalf("error leaked credentials: %v", err)
	}
}

func TestBaiduPanAuthenticatesStaleSessionAndReportsHealth(t *testing.T) {
	var whoCalls atomic.Int32
	var loginCalls atomic.Int32
	runner := &fakeBaiduRunner{outputFn: func(_ context.Context, command baidupan.Command) ([]byte, error) {
		switch commandName(command) {
		case "who":
			if whoCalls.Add(1) == 1 {
				return []byte("当前帐号 uid: 0, 用户名: , 性别: , 年龄: 0.0\n"), nil
			}
			return []byte("当前帐号 uid: 10001, 用户名: fixture-user, 性别: , 年龄: 0.0\n"), nil
		case "login":
			loginCalls.Add(1)
			if strings.Contains(strings.Join(command.Args, " "), testBDUSS) || strings.Contains(strings.Join(command.Args, " "), testSTOKEN) {
				return nil, errors.New("credentials were exposed in login argv")
			}
			if !command.Interactive || !strings.Contains(command.Input, testBDUSS) || !strings.Contains(command.Input, testSTOKEN) {
				return nil, errors.New("credentials were not passed over protected stdin")
			}
			return []byte("登录成功\n"), nil
		}
		return nil, nil
	}}
	fs := newBaiduFileSystem(t, runner, "account")
	if _, err := fs.Stat(context.Background(), remotefs.Reference{Path: "/shows/episode 01.mp4"}); err != nil {
		t.Fatalf("stat after login: %v", err)
	}
	if whoCalls.Load() != 2 || loginCalls.Load() != 1 {
		t.Fatalf("who/login calls = %d/%d, want 2/1", whoCalls.Load(), loginCalls.Load())
	}
	checker, ok := fs.(remotefs.HealthChecker)
	if !ok {
		t.Fatal("baidupan does not implement HealthChecker")
	}
	health, err := checker.Check(context.Background())
	if err != nil || health.Status != remotefs.HealthHealthy || health.Version != "v4.0.0" {
		t.Fatalf("health/error = %+v / %v", health, err)
	}
}

func TestBaiduPanRejectsUnexpectedProviderUID(t *testing.T) {
	var metaCalls atomic.Int32
	runner := &fakeBaiduRunner{outputFn: func(_ context.Context, command baidupan.Command) ([]byte, error) {
		if commandName(command) == "meta" {
			metaCalls.Add(1)
		}
		return nil, nil
	}}
	fs := newBaiduFileSystem(t, runner, "account", func(cfg *baiduTestConfig) {
		cfg.session.ExpectedProviderUID = "99999"
	})
	_, err := fs.Stat(context.Background(), remotefs.Reference{Path: "/shows/episode 01.mp4"})
	if !errors.Is(err, remotefs.ErrIdentityMismatch) {
		t.Fatalf("Stat() error = %v, want ErrIdentityMismatch", err)
	}
	if metaCalls.Load() != 0 {
		t.Fatalf("meta calls = %d, want 0", metaCalls.Load())
	}
}

func TestBaiduPanCredentialRevisionUsesDistinctSession(t *testing.T) {
	runner := &fakeBaiduRunner{}
	configRoot := t.TempDir()
	first := newBaiduFileSystem(t, runner, "account", func(cfg *baiduTestConfig) {
		cfg.runtime.ConfigRoot = configRoot
		cfg.session.CredentialRevision = "revision-1"
	})
	second := newBaiduFileSystem(t, runner, "account", func(cfg *baiduTestConfig) {
		cfg.runtime.ConfigRoot = configRoot
		cfg.session.CredentialRevision = "revision-2"
	})
	if _, err := first.Stat(context.Background(), remotefs.Reference{Path: "/shows/episode 01.mp4"}); err != nil {
		t.Fatal(err)
	}
	if _, err := second.Stat(context.Background(), remotefs.Reference{Path: "/shows/episode 01.mp4"}); err != nil {
		t.Fatal(err)
	}
	dirs := make(map[string]struct{})
	for _, command := range runner.snapshot() {
		if configDir := envValue(command.Env, "BAIDUPCS_GO_CONFIG_DIR"); configDir != "" {
			dirs[configDir] = struct{}{}
		}
	}
	if len(dirs) != 2 {
		t.Fatalf("session directories = %v, want 2 revisions", dirs)
	}
}

func TestBaiduPanReportsProviderNeutralQuota(t *testing.T) {
	fs := newBaiduFileSystem(t, &fakeBaiduRunner{}, "quota-account")
	reader, ok := fs.(remotefs.QuotaReader)
	if !ok {
		t.Fatal("baidupan does not expose QuotaReader")
	}
	quota, err := reader.Quota(context.Background())
	if err != nil {
		t.Fatalf("quota: %v", err)
	}
	if quota.TotalBytes != 2<<40 || quota.UsedBytes != 512<<30 {
		t.Fatalf("quota = %+v", quota)
	}
}

func TestBaiduPanRejectsUnreliableQuotaSnapshot(t *testing.T) {
	runner := &fakeBaiduRunner{outputFn: func(_ context.Context, command baidupan.Command) ([]byte, error) {
		if commandName(command) == "quota" {
			return []byte("用户名: fixture-user, 总空间: 0B, 已用空间: 0B, 比率: NaN%\n"), nil
		}
		return nil, nil
	}}
	fs := newBaiduFileSystem(t, runner, "quota-account")
	_, err := fs.(remotefs.QuotaReader).Quota(context.Background())
	if !errors.Is(err, remotefs.ErrTemporary) {
		t.Fatalf("Quota() error = %v, want ErrTemporary", err)
	}
}

func TestBaiduPanHealthRevalidatesAuthenticatedSession(t *testing.T) {
	var whoCalls atomic.Int32
	runner := &fakeBaiduRunner{outputFn: func(_ context.Context, command baidupan.Command) ([]byte, error) {
		if commandName(command) == "who" {
			if whoCalls.Add(1) == 1 {
				return []byte("当前帐号 uid: 10001, 用户名: fixture-user, 性别: , 年龄: 0.0\n"), nil
			}
			return []byte("当前帐号 uid: 0, 用户名: , 性别: , 年龄: 0.0\n"), nil
		}
		return nil, nil
	}}
	fs := newBaiduFileSystem(t, runner, "account", func(cfg *baiduTestConfig) {
		cfg.session.BDUSS = ""
		cfg.session.STOKEN = ""
	})
	if _, err := fs.Stat(context.Background(), remotefs.Reference{Path: "/shows/episode 01.mp4"}); err != nil {
		t.Fatalf("initial stat: %v", err)
	}
	checker := fs.(remotefs.HealthChecker)
	health, err := checker.Check(context.Background())
	if !errors.Is(err, remotefs.ErrUnauthenticated) || health.Status != remotefs.HealthDegraded {
		t.Fatalf("stale health/error = %+v / %v", health, err)
	}
	if whoCalls.Load() != 2 {
		t.Fatalf("who calls = %d, want 2", whoCalls.Load())
	}
}

func TestBaiduPanUnauthenticatedOutputInvalidatesCachedSession(t *testing.T) {
	var whoCalls atomic.Int32
	var metaCalls atomic.Int32
	runner := &fakeBaiduRunner{outputFn: func(_ context.Context, command baidupan.Command) ([]byte, error) {
		switch commandName(command) {
		case "who":
			whoCalls.Add(1)
			return []byte("当前帐号 uid: 10001, 用户名: fixture-user, 性别: , 年龄: 0.0\n"), nil
		case "meta":
			if metaCalls.Add(1) == 1 {
				return []byte(loadFixturePath("testdata/unauthenticated.txt")), nil
			}
		}
		return nil, nil
	}}
	fs := newBaiduFileSystem(t, runner, "account")
	ref := remotefs.Reference{Path: "/shows/episode 01.mp4"}
	if _, err := fs.Stat(context.Background(), ref); !errors.Is(err, remotefs.ErrUnauthenticated) {
		t.Fatalf("first stat error = %v", err)
	}
	if _, err := fs.Stat(context.Background(), ref); err != nil {
		t.Fatalf("second stat after revalidation: %v", err)
	}
	if whoCalls.Load() != 2 {
		t.Fatalf("who calls = %d, want 2", whoCalls.Load())
	}
}

func TestBaiduPanParseShareNormalizesPassword(t *testing.T) {
	result, err := baidupan.ParseShare(remotefs.ShareRequest{
		URL: "https://pan.baidu.com/s/1abcdefghijklmnopqrstuv?pwd=a1b2",
	})
	if err != nil {
		t.Fatalf("parse share: %v", err)
	}
	if result.Feature == "" || result.Password != "a1b2" || strings.Contains(result.URL, "pwd=") {
		t.Fatalf("parsed share = %+v", result)
	}
}

func TestBaiduPanParseShareRejectsUnsafeURLsAndPasswords(t *testing.T) {
	for _, request := range []remotefs.ShareRequest{
		{URL: "http://pan.baidu.com/s/1abcdefghijklmnopqrstuv", Password: "a1b2"},
		{URL: "https://user@pan.baidu.com/s/1abcdefghijklmnopqrstuv", Password: "a1b2"},
		{URL: "https://pan.baidu.com/redirect/s/1abcdefgh", Password: "a1b2"},
		{URL: "https://pan.baidu.com/s/1abc%20def", Password: "a1b2"},
	} {
		if _, err := baidupan.ParseShare(request); !errors.Is(err, baidupan.ErrShareInvalid) {
			t.Fatalf("ParseShare(%q) error = %v", request.URL, err)
		}
	}
	_, err := baidupan.ParseShare(remotefs.ShareRequest{
		URL: "https://pan.baidu.com/s/1abcdefghijklmnopqrstuv", Password: "bad",
	})
	if !errors.Is(err, baidupan.ErrSharePasswordInvalid) {
		t.Fatalf("invalid password error = %v", err)
	}
}

func TestBaiduPanStageShareClassifiesBusinessOutput(t *testing.T) {
	tests := []struct {
		fixture   string
		want      error
		duplicate bool
	}{
		{"transfer_success.txt", nil, false},
		{"transfer_duplicate.txt", remotefs.ErrConflict, true},
		{"share_password_invalid.txt", baidupan.ErrSharePasswordInvalid, false},
		{"share_expired.txt", baidupan.ErrShareExpired, false},
		{"quota_exceeded.txt", remotefs.ErrQuotaExceeded, false},
	}
	for _, test := range tests {
		t.Run(test.fixture, func(t *testing.T) {
			runner := &fakeBaiduRunner{outputFn: func(_ context.Context, command baidupan.Command) ([]byte, error) {
				if commandName(command) == "transfer" {
					return []byte(loadFixturePath("testdata/" + test.fixture)), nil
				}
				return nil, nil
			}}
			fs := newBaiduFileSystem(t, runner, "account")
			result, err := baiduStager(t, fs).StageShare(context.Background(), remotefs.StageRequest{
				Share:       remotefs.ShareRequest{URL: "https://pan.baidu.com/s/1abcdefghijklmnopqrstuv", Password: "a1b2"},
				Destination: remotefs.Reference{Path: "/staging/task-1"},
			})
			if test.want == nil {
				if err != nil || result.Name != "sample-show" {
					t.Fatalf("StageShare result/error = %+v / %v", result, err)
				}
				commands := runner.snapshot()
				for _, command := range commands {
					if commandName(command) != "transfer" {
						continue
					}
					if strings.Contains(strings.Join(command.Args, " "), "a1b2") || !command.Interactive || !strings.Contains(command.Input, "a1b2") {
						t.Fatalf("transfer password was not confined to stdin: args=%+v interactive=%t", command.Args, command.Interactive)
					}
				}
				return
			}
			if !errors.Is(err, test.want) {
				t.Fatalf("StageShare error = %v, want %v", err, test.want)
			}
			if test.duplicate && !result.Duplicate {
				t.Fatalf("duplicate result = %+v", result)
			}
		})
	}
}

func TestBaiduPanStageShareAcceptsExistingDestination(t *testing.T) {
	runner := &fakeBaiduRunner{outputFn: func(_ context.Context, command baidupan.Command) ([]byte, error) {
		if commandName(command) == "mkdir" {
			return []byte("文件已存在\n"), nil
		}
		return nil, nil
	}}
	fs := newBaiduFileSystem(t, runner, "account")
	if _, err := baiduStager(t, fs).StageShare(context.Background(), remotefs.StageRequest{
		Share:       remotefs.ShareRequest{URL: "https://pan.baidu.com/s/1abcdefghijklmnopqrstuv", Password: "a1b2"},
		Destination: remotefs.Reference{Path: "/staging/task"},
	}); err != nil {
		t.Fatalf("StageShare existing destination: %v", err)
	}
	commands := statefulCommands(runner.snapshot())
	if strings.Join(commands, ",") != "mkdir:/staging/task,cd:/staging/task,transfer" {
		t.Fatalf("stateful command order = %v", commands)
	}
}

func TestBaiduPanStageShareRejectsProviderNameTraversal(t *testing.T) {
	runner := &fakeBaiduRunner{outputFn: func(_ context.Context, command baidupan.Command) ([]byte, error) {
		if commandName(command) == "transfer" {
			return []byte("分享链接转存到网盘成功, 保存了../../escape到当前目录\n"), nil
		}
		return nil, nil
	}}
	fs := newBaiduFileSystem(t, runner, "account")
	_, err := baiduStager(t, fs).StageShare(context.Background(), remotefs.StageRequest{
		Share:       remotefs.ShareRequest{URL: "https://pan.baidu.com/s/1abcdefghijklmnopqrstuv", Password: "a1b2"},
		Destination: remotefs.Reference{Path: "/staging/task"},
	})
	if !errors.Is(err, remotefs.ErrPermissionDenied) {
		t.Fatalf("StageShare error = %v, want ErrPermissionDenied", err)
	}
}

func TestBaiduPanTwoAccountsRunConcurrentlyWithIsolatedConfigDirs(t *testing.T) {
	metaStarted := make(chan struct{}, 2)
	release := make(chan struct{})
	runner := &fakeBaiduRunner{outputFn: func(ctx context.Context, command baidupan.Command) ([]byte, error) {
		if commandName(command) == "meta" {
			metaStarted <- struct{}{}
			select {
			case <-release:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			return []byte(loadFixturePath("testdata/meta_success.txt")), nil
		}
		return nil, nil
	}}
	first := newBaiduFileSystem(t, runner, "account-one")
	second := newBaiduFileSystem(t, runner, "account-two")
	done := make(chan error, 2)
	for _, fs := range []remotefs.FileSystem{first, second} {
		go func() {
			_, err := fs.Stat(context.Background(), remotefs.Reference{Path: "/shows/episode 01.mp4"})
			done <- err
		}()
	}
	for range 2 {
		select {
		case <-metaStarted:
		case <-time.After(time.Second):
			t.Fatal("different accounts did not execute concurrently")
		}
	}
	close(release)
	for range 2 {
		if err := <-done; err != nil {
			t.Fatalf("concurrent stat: %v", err)
		}
	}
	configDirs := map[string]bool{}
	for _, command := range runner.snapshot() {
		configDir := envValue(command.Env, "BAIDUPCS_GO_CONFIG_DIR")
		if configDir == "" {
			if commandName(command) == "--version" {
				continue
			}
			t.Fatalf("session command missing isolated config dir: %+v", command)
		}
		configDirs[configDir] = true
	}
	if len(configDirs) != 2 || runner.maxActive.Load() < 2 {
		t.Fatalf("config dirs/max concurrency = %v/%d", configDirs, runner.maxActive.Load())
	}
}

func TestBaiduPanSerializesStatefulStageSharePerAccount(t *testing.T) {
	firstTransfer := make(chan struct{})
	release := make(chan struct{})
	var transferCalls atomic.Int32
	runner := &fakeBaiduRunner{outputFn: func(ctx context.Context, command baidupan.Command) ([]byte, error) {
		if commandName(command) == "transfer" && transferCalls.Add(1) == 1 {
			close(firstTransfer)
			select {
			case <-release:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			return []byte(loadFixturePath("testdata/transfer_success.txt")), nil
		}
		return nil, nil
	}}
	fs := newBaiduFileSystem(t, runner, "account")
	done := make(chan error, 2)
	stage := func(destination string) {
		_, err := baiduStager(t, fs).StageShare(context.Background(), remotefs.StageRequest{
			Share:       remotefs.ShareRequest{URL: "https://pan.baidu.com/s/1abcdefghijklmnopqrstuv", Password: "a1b2"},
			Destination: remotefs.Reference{Path: destination},
		})
		done <- err
	}
	go stage("/stage/first")
	select {
	case <-firstTransfer:
	case <-time.After(time.Second):
		t.Fatal("first transfer did not start")
	}
	go stage("/stage/second")
	time.Sleep(50 * time.Millisecond)
	if got := countCommands(runner.snapshot(), "cd"); got != 1 {
		t.Fatalf("cd calls while first transfer blocked = %d, want 1", got)
	}
	close(release)
	for range 2 {
		if err := <-done; err != nil {
			t.Fatalf("stage share: %v", err)
		}
	}
	commands := statefulCommands(runner.snapshot())
	if strings.Join(commands, ",") != "mkdir:/stage/first,cd:/stage/first,transfer,mkdir:/stage/second,cd:/stage/second,transfer" {
		t.Fatalf("stateful command order = %v", commands)
	}
}

func TestBaiduPanSerializesStatefulStageShareAcrossInstances(t *testing.T) {
	firstTransfer := make(chan struct{})
	release := make(chan struct{})
	var transferCalls atomic.Int32
	runner := &fakeBaiduRunner{outputFn: func(ctx context.Context, command baidupan.Command) ([]byte, error) {
		if commandName(command) == "transfer" && transferCalls.Add(1) == 1 {
			close(firstTransfer)
			select {
			case <-release:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			return []byte(loadFixturePath("testdata/transfer_success.txt")), nil
		}
		return nil, nil
	}}
	configRoot := t.TempDir()
	useSharedConfigRoot := func(cfg *baiduTestConfig) { cfg.runtime.ConfigRoot = configRoot }
	first := newBaiduFileSystem(t, runner, "shared-account", useSharedConfigRoot)
	second := newBaiduFileSystem(t, runner, "shared-account", useSharedConfigRoot)
	done := make(chan error, 2)
	stage := func(fs remotefs.FileSystem, destination string) {
		_, err := baiduStager(t, fs).StageShare(context.Background(), remotefs.StageRequest{
			Share:       remotefs.ShareRequest{URL: "https://pan.baidu.com/s/1abcdefghijklmnopqrstuv", Password: "a1b2"},
			Destination: remotefs.Reference{Path: destination},
		})
		done <- err
	}
	go stage(first, "/stage/first")
	select {
	case <-firstTransfer:
	case <-time.After(time.Second):
		t.Fatal("first transfer did not start")
	}
	go stage(second, "/stage/second")
	time.Sleep(50 * time.Millisecond)
	if got := countCommands(runner.snapshot(), "cd"); got != 1 {
		t.Fatalf("cd calls while first instance transfer blocked = %d, want 1", got)
	}
	close(release)
	for range 2 {
		if err := <-done; err != nil {
			t.Fatalf("stage share: %v", err)
		}
	}
	commands := statefulCommands(runner.snapshot())
	if strings.Join(commands, ",") != "mkdir:/stage/first,cd:/stage/first,transfer,mkdir:/stage/second,cd:/stage/second,transfer" {
		t.Fatalf("stateful command order = %v", commands)
	}
}

func TestBaiduPanSerializesStatefulStageShareAcrossProcesses(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "transfers.log")
	binaryPath := filepath.Join(t.TempDir(), "BaiduPCS-Go")
	script := fmt.Sprintf(`#!/bin/sh
if [ "$1" = "--version" ]; then
  echo "BaiduPCS-Go version v4.0.0"
elif [ "$1" = "who" ]; then
  echo "当前帐号 uid: 10001, 用户名: fixture-user, 性别: , 年龄: 0.0"
elif [ "$1" = "mkdir" ]; then
  echo "创建目录成功"
elif [ "$1" = "cd" ]; then
  echo "改变工作目录成功"
elif [ "$#" -eq 0 ]; then
  echo "start $$" >> %q
  /bin/sleep 0.2
  echo "end $$" >> %q
  echo "保存了fixture.mp4到当前目录"
fi
`, logPath, logPath)
	if err := os.WriteFile(binaryPath, []byte(script), 0o700); err != nil {
		t.Fatalf("write helper binary: %v", err)
	}
	configRoot := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	commands := make([]*exec.Cmd, 2)
	outputs := make([]bytes.Buffer, len(commands))
	for index := range commands {
		command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestBaiduPanCrossProcessStageHelper$")
		command.Env = append(os.Environ(),
			"SDKIT_BAIDUPAN_PROCESS_HELPER=1",
			"SDKIT_BAIDUPAN_PROCESS_BINARY="+binaryPath,
			"SDKIT_BAIDUPAN_PROCESS_CONFIG_ROOT="+configRoot,
		)
		command.Stdout = &outputs[index]
		command.Stderr = &outputs[index]
		commands[index] = command
		if err := command.Start(); err != nil {
			t.Fatalf("start helper %d: %v", index, err)
		}
	}
	for index, command := range commands {
		if err := command.Wait(); err != nil {
			t.Fatalf("helper %d: %v\n%s", index, err, outputs[index].String())
		}
	}
	lines := strings.Fields(strings.TrimSpace(loadFixturePath(logPath)))
	if len(lines) != 8 {
		t.Fatalf("transfer log fields = %v", lines)
	}
	if lines[0] != "start" || lines[2] != "end" || lines[4] != "start" || lines[6] != "end" {
		t.Fatalf("cross-process transfers overlapped: %v", lines)
	}
}

func TestBaiduPanCrossProcessStageHelper(t *testing.T) {
	if os.Getenv("SDKIT_BAIDUPAN_PROCESS_HELPER") != "1" {
		t.Skip("cross-process helper")
	}
	runtime, err := baidupan.NewRuntime(context.Background(), baidupan.RuntimeConfig{
		BinaryPath: os.Getenv("SDKIT_BAIDUPAN_PROCESS_BINARY"),
		MinVersion: "v4.0.0", MaxVersion: "v4.0.0",
		ConfigRoot: os.Getenv("SDKIT_BAIDUPAN_PROCESS_CONFIG_ROOT"),
	})
	if err != nil {
		t.Fatal(err)
	}
	fs, err := runtime.Open(context.Background(), baidupan.SessionConfig{SessionKey: "shared-process-account"})
	if err != nil {
		t.Fatal(err)
	}
	defer fs.Close()
	_, err = fs.(remotefs.ShareStager).StageShare(context.Background(), remotefs.StageRequest{
		Share:       remotefs.ShareRequest{URL: "https://pan.baidu.com/s/1abcdefghijklmnopqrstuv", Password: "a1b2"},
		Destination: remotefs.Reference{Path: "/stage/process"},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestBaiduPanDownloadMapsProgressAndFinalPath(t *testing.T) {
	runner := &fakeBaiduRunner{}
	runner.streamFn = func(ctx context.Context, command baidupan.Command, sink execx.Sink) (execx.Result, error) {
		saveDir := commandArgValue(command.Args, "--saveto=")
		if saveDir == "" {
			return execx.Result{}, errors.New("missing saveto")
		}
		finalPath := filepath.Join(saveDir, "episode 01.mp4")
		if err := os.WriteFile(finalPath, []byte("0123456789"), 0o600); err != nil {
			return execx.Result{}, err
		}
		for _, line := range []string{
			"[1] ↓ 5B/10B 5B/s in 1s, left 1s",
			"[1] ↓ 10B/10B 5B/s in 2s, left 0s",
			"[1] 下载完成, 保存位置: " + finalPath,
		} {
			if err := sink.WriteCommandEvent(ctx, execx.Event{Stream: execx.StreamStdout, Data: []byte(line), Text: line}); err != nil {
				return execx.Result{Command: command.Name, Args: command.Args, ExitCode: -1}, err
			}
		}
		return execx.Result{Command: command.Name, Args: command.Args, ExitCode: 0}, nil
	}
	fs := newBaiduFileSystem(t, runner, "account")
	target := filepath.Join(t.TempDir(), "final.mp4")
	var progress []remotefs.Progress
	result, err := fs.(remotefs.Downloader).Download(context.Background(), remotefs.DownloadRequest{
		Reference: remotefs.Reference{Path: "/shows/episode 01.mp4"}, Destination: target,
	}, remotefs.ProgressSinkFunc(func(_ context.Context, event remotefs.Progress) error {
		progress = append(progress, event)
		return nil
	}))
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	if result.Path != target || result.BytesWritten != 10 {
		t.Fatalf("download result = %+v", result)
	}
	if len(progress) < 3 || progress[len(progress)-1].Phase != remotefs.ProgressFinalizing || progress[len(progress)-1].TransferredBytes != 10 {
		t.Fatalf("progress = %+v", progress)
	}
}

func TestBaiduPanDownloadToleratesRoundedProviderTotal(t *testing.T) {
	runner := &fakeBaiduRunner{}
	runner.streamFn = func(ctx context.Context, command baidupan.Command, sink execx.Sink) (execx.Result, error) {
		saveDir := commandArgValue(command.Args, "--saveto=")
		if saveDir == "" {
			return execx.Result{}, errors.New("missing saveto")
		}
		finalPath := filepath.Join(saveDir, "episode 01.mp4")
		if err := os.WriteFile(finalPath, []byte("0123456789"), 0o600); err != nil {
			return execx.Result{}, err
		}
		for _, line := range []string{
			"[1] ↓ 5B/9B 5B/s in 1s, left 1s",
			"[1] ↓ 10B/9B 5B/s in 2s, left 0s",
			"[1] 下载完成, 保存位置: " + finalPath,
		} {
			if err := sink.WriteCommandEvent(ctx, execx.Event{Stream: execx.StreamStdout, Data: []byte(line), Text: line}); err != nil {
				return execx.Result{Command: command.Name, Args: command.Args, ExitCode: -1}, err
			}
		}
		return execx.Result{Command: command.Name, Args: command.Args, ExitCode: 0}, nil
	}
	fs := newBaiduFileSystem(t, runner, "account")
	target := filepath.Join(t.TempDir(), "final.mp4")
	var progress []remotefs.Progress
	result, err := fs.(remotefs.Downloader).Download(context.Background(), remotefs.DownloadRequest{
		Reference: remotefs.Reference{Path: "/shows/episode 01.mp4"}, Destination: target,
	}, remotefs.ProgressSinkFunc(func(_ context.Context, event remotefs.Progress) error {
		progress = append(progress, event)
		return nil
	}))
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	if result.Path != target || result.BytesWritten != 10 {
		t.Fatalf("download result = %+v", result)
	}
	for _, event := range progress {
		if event.TotalBytes > 0 && event.TransferredBytes > event.TotalBytes {
			t.Fatalf("progress reported beyond the reported total: %+v", event)
		}
	}
	if len(progress) < 3 || progress[len(progress)-1].Phase != remotefs.ProgressFinalizing || progress[len(progress)-1].TransferredBytes != 9 {
		t.Fatalf("progress = %+v", progress)
	}
}

func TestBaiduPanDownloadDenyDoesNotReplaceRacingDestination(t *testing.T) {
	runner := &fakeBaiduRunner{}
	runner.streamFn = func(ctx context.Context, command baidupan.Command, sink execx.Sink) (execx.Result, error) {
		saveDir := commandArgValue(command.Args, "--saveto=")
		finalPath := filepath.Join(saveDir, "episode 01.mp4")
		if err := os.WriteFile(finalPath, []byte("0123456789"), 0o600); err != nil {
			return execx.Result{}, err
		}
		line := "[1] 下载完成, 保存位置: " + finalPath
		if err := sink.WriteCommandEvent(ctx, execx.Event{Text: line, Data: []byte(line)}); err != nil {
			return execx.Result{}, err
		}
		return execx.Result{ExitCode: 0}, nil
	}
	fs := newBaiduFileSystem(t, runner, "account")
	destination := filepath.Join(t.TempDir(), "destination.bin")
	_, err := fs.(remotefs.Downloader).Download(context.Background(), remotefs.DownloadRequest{
		Reference: remotefs.Reference{Path: "/shows/episode 01.mp4"}, Destination: destination, Overwrite: remotefs.OverwriteDeny,
	}, remotefs.ProgressSinkFunc(func(_ context.Context, progress remotefs.Progress) error {
		if progress.Phase == remotefs.ProgressFinalizing {
			return os.WriteFile(destination, []byte("racing-writer"), 0o600)
		}
		return nil
	}))
	if !errors.Is(err, remotefs.ErrConflict) {
		t.Fatalf("Download() error = %v, want ErrConflict", err)
	}
	data, readErr := os.ReadFile(destination)
	if readErr != nil || string(data) != "racing-writer" {
		t.Fatalf("destination = %q, %v", data, readErr)
	}
}

func TestBaiduPanConcurrentOverwriteDenyCommitsOnce(t *testing.T) {
	ready := make(chan struct{}, 2)
	release := make(chan struct{})
	runner := &fakeBaiduRunner{}
	runner.streamFn = func(ctx context.Context, command baidupan.Command, sink execx.Sink) (execx.Result, error) {
		saveDir := commandArgValue(command.Args, "--saveto=")
		finalPath := filepath.Join(saveDir, "episode 01.mp4")
		if err := os.WriteFile(finalPath, []byte("0123456789"), 0o600); err != nil {
			return execx.Result{}, err
		}
		ready <- struct{}{}
		<-release
		line := "[1] 下载完成, 保存位置: " + finalPath
		if err := sink.WriteCommandEvent(ctx, execx.Event{Stream: execx.StreamStdout, Data: []byte(line), Text: line}); err != nil {
			return execx.Result{}, err
		}
		return execx.Result{ExitCode: 0}, nil
	}
	fs := newBaiduFileSystem(t, runner, "account")
	destination := filepath.Join(t.TempDir(), "destination.bin")

	errorsOut := make(chan error, 2)
	for range 2 {
		go func() {
			_, err := fs.(remotefs.Downloader).Download(context.Background(), remotefs.DownloadRequest{
				Reference: remotefs.Reference{Path: "/shows/episode 01.mp4"}, Destination: destination, Overwrite: remotefs.OverwriteDeny,
			}, nil)
			errorsOut <- err
		}()
	}
	for range 2 {
		select {
		case <-ready:
		case <-time.After(time.Second):
			close(release)
			t.Fatal("concurrent downloads did not reach transfer")
		}
	}
	close(release)
	successes := 0
	conflicts := 0
	for range 2 {
		err := <-errorsOut
		switch {
		case err == nil:
			successes++
		case errors.Is(err, remotefs.ErrConflict):
			conflicts++
		default:
			t.Fatalf("Download() error = %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("successes = %d, conflicts = %d, want 1/1", successes, conflicts)
	}
	data, err := os.ReadFile(destination)
	if err != nil || string(data) != "0123456789" {
		t.Fatalf("destination = %q, err = %v", data, err)
	}
}

func TestBaiduPanDownloadRejectsSavePathEscapeAndPropagatesSinkError(t *testing.T) {
	outside := filepath.Join(t.TempDir(), "outside.mp4")
	runner := &fakeBaiduRunner{}
	runner.streamFn = func(ctx context.Context, command baidupan.Command, sink execx.Sink) (execx.Result, error) {
		line := "[1] 下载完成, 保存位置: " + outside
		if err := sink.WriteCommandEvent(ctx, execx.Event{Stream: execx.StreamStdout, Data: []byte(line), Text: line}); err != nil {
			return execx.Result{}, err
		}
		return execx.Result{ExitCode: 0}, nil
	}
	fs := newBaiduFileSystem(t, runner, "account")
	_, err := fs.(remotefs.Downloader).Download(context.Background(), remotefs.DownloadRequest{
		Reference: remotefs.Reference{Path: "/shows/episode 01.mp4"}, Destination: filepath.Join(t.TempDir(), "target.mp4"),
	}, nil)
	if !errors.Is(err, remotefs.ErrPermissionDenied) {
		t.Fatalf("save path escape error = %v", err)
	}

	want := errors.New("stop progress")
	runner.streamFn = func(ctx context.Context, command baidupan.Command, sink execx.Sink) (execx.Result, error) {
		line := "[1] ↓ 5B/10B 5B/s in 1s, left 1s"
		return execx.Result{}, sink.WriteCommandEvent(ctx, execx.Event{Stream: execx.StreamStdout, Data: []byte(line), Text: line})
	}
	_, err = fs.(remotefs.Downloader).Download(context.Background(), remotefs.DownloadRequest{
		Reference: remotefs.Reference{Path: "/shows/episode 01.mp4"}, Destination: filepath.Join(t.TempDir(), "target.mp4"),
	}, remotefs.ProgressSinkFunc(func(context.Context, remotefs.Progress) error { return want }))
	if !errors.Is(err, want) {
		t.Fatalf("sink error = %v, want %v", err, want)
	}
	if errors.Is(err, remotefs.ErrTemporary) {
		t.Fatalf("sink error was misclassified as provider temporary failure: %v", err)
	}
}

func TestBaiduPanDownloadRejectsNonMonotonicProviderProgress(t *testing.T) {
	runner := &fakeBaiduRunner{streamFn: func(ctx context.Context, _ baidupan.Command, sink execx.Sink) (execx.Result, error) {
		for _, line := range []string{
			"[1] ↓ 8B/10B 5B/s in 1s, left 1s",
			"[1] ↓ 4B/10B 5B/s in 2s, left 1s",
		} {
			if err := sink.WriteCommandEvent(ctx, execx.Event{Text: line, Data: []byte(line)}); err != nil {
				return execx.Result{}, err
			}
		}
		return execx.Result{}, nil
	}}
	fs := newBaiduFileSystem(t, runner, "account")
	destination := filepath.Join(t.TempDir(), "target.mp4")
	_, err := fs.(remotefs.Downloader).Download(context.Background(), remotefs.DownloadRequest{
		Reference: remotefs.Reference{Path: "/shows/episode 01.mp4"}, Destination: destination,
	}, nil)
	if !errors.Is(err, remotefs.ErrProtocol) {
		t.Fatalf("non-monotonic progress error = %v", err)
	}
	if _, statErr := os.Stat(destination); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("destination stat error = %v", statErr)
	}
}

func TestBaiduPanRejectsInvalidPathsOptionsAndProviderOutput(t *testing.T) {
	runner := &fakeBaiduRunner{}
	fs := newBaiduFileSystem(t, runner, "account")
	if _, err := fs.Stat(nil, remotefs.Reference{Path: "/shows/file.mp4"}); !errors.Is(err, remotefs.ErrNilContext) {
		t.Fatalf("nil context error = %v", err)
	}
	if _, err := fs.Stat(context.Background(), remotefs.Reference{Path: "relative/file.mp4"}); !errors.Is(err, remotefs.ErrInvalidReference) {
		t.Fatalf("relative path error = %v", err)
	}
	for _, opts := range []remotefs.ListOptions{
		{PageSize: -1}, {PageSize: 1001}, {SortBy: "invalid"}, {Order: "invalid"},
	} {
		if _, err := fs.List(context.Background(), remotefs.Reference{Path: "/shows"}, opts); !errors.Is(err, remotefs.ErrInvalidOption) {
			t.Fatalf("List(%+v) error = %v", opts, err)
		}
	}

	runner.outputFn = func(_ context.Context, command baidupan.Command) ([]byte, error) {
		switch commandName(command) {
		case "ls":
			return []byte("  0  10B 2026-08-25 12:00:00 网络错误.mp4\n文件总数: 1, 目录总数: 0\n"), nil
		case "meta":
			fixture := loadFixturePath("testdata/meta_success.txt")
			fixture = strings.ReplaceAll(fixture, "episode 01.mp4", "网络错误.mp4")
			return []byte(fixture), nil
		}
		return nil, nil
	}
	page, err := fs.List(context.Background(), remotefs.Reference{Path: "/shows"}, remotefs.ListOptions{})
	if err != nil || len(page.Entries) != 1 || page.Entries[0].Name != "网络错误.mp4" {
		t.Fatalf("keyword filename list = %+v, %v", page, err)
	}

	runner.outputFn = func(_ context.Context, command baidupan.Command) ([]byte, error) {
		if commandName(command) == "ls" {
			return []byte("  0  10B 2026-08-25 12:00:00 ../escape.mp4\n文件总数: 1, 目录总数: 0\n"), nil
		}
		return nil, nil
	}
	if _, err := fs.List(context.Background(), remotefs.Reference{Path: "/shows"}, remotefs.ListOptions{}); !errors.Is(err, remotefs.ErrProtocol) {
		t.Fatalf("unsafe child error = %v", err)
	}

	runner.outputFn = func(_ context.Context, command baidupan.Command) ([]byte, error) {
		if commandName(command) == "meta" {
			fixture := strings.Replace(loadFixturePath("testdata/meta_success.txt"), "2026-08-22 09:59:00", "invalid-time", 1)
			return []byte(fixture), nil
		}
		return nil, nil
	}
	if _, err := fs.Stat(context.Background(), remotefs.Reference{Path: "/shows/episode 01.mp4"}); !errors.Is(err, remotefs.ErrProtocol) {
		t.Fatalf("invalid metadata time error = %v", err)
	}
}

func TestBaiduPanDownloadHonorsContextCancellation(t *testing.T) {
	started := make(chan struct{})
	runner := &fakeBaiduRunner{}
	runner.streamFn = func(ctx context.Context, _ baidupan.Command, _ execx.Sink) (execx.Result, error) {
		close(started)
		<-ctx.Done()
		return execx.Result{ExitCode: -1}, ctx.Err()
	}
	fs := newBaiduFileSystem(t, runner, "account")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := fs.(remotefs.Downloader).Download(ctx, remotefs.DownloadRequest{
			Reference: remotefs.Reference{Path: "/shows/episode 01.mp4"}, Destination: filepath.Join(t.TempDir(), "target.mp4"),
		}, nil)
		done <- err
	}()
	<-started
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("download cancellation error = %v", err)
	}
}

func TestBaiduPanDownloadPartialFilePolicy(t *testing.T) {
	for _, keep := range []bool{false, true} {
		t.Run(fmt.Sprintf("keep=%t", keep), func(t *testing.T) {
			runner := &fakeBaiduRunner{}
			runner.streamFn = func(_ context.Context, command baidupan.Command, _ execx.Sink) (execx.Result, error) {
				saveDir := commandArgValue(command.Args, "--saveto=")
				if err := os.WriteFile(filepath.Join(saveDir, "partial.download"), []byte("partial"), 0o600); err != nil {
					return execx.Result{}, err
				}
				return execx.Result{ExitCode: 1}, errors.New("download interrupted")
			}
			fs := newBaiduFileSystem(t, runner, "account")
			target := filepath.Join(t.TempDir(), "target.mp4")
			policy := remotefs.PartialFileRemove
			if keep {
				policy = remotefs.PartialFileKeep
			}
			_, err := fs.(remotefs.Downloader).Download(context.Background(), remotefs.DownloadRequest{
				Reference: remotefs.Reference{Path: "/shows/episode 01.mp4"}, Destination: target, PartialFile: policy,
			}, nil)
			if err == nil {
				t.Fatal("interrupted download error = nil")
			}
			_, statErr := os.Stat(target + ".partial")
			if keep && statErr != nil {
				t.Fatalf("kept partial stat error = %v", statErr)
			}
			if !keep && !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("removed partial stat error = %v", statErr)
			}
		})
	}
}

func TestBaiduPanConcurrentKeepPartialHasSingleOwner(t *testing.T) {
	ready := make(chan struct{}, 2)
	release := make(chan struct{})
	var sequence atomic.Int32
	runner := &fakeBaiduRunner{streamFn: func(_ context.Context, command baidupan.Command, _ execx.Sink) (execx.Result, error) {
		content := fmt.Sprintf("partial-%d", sequence.Add(1))
		saveDir := commandArgValue(command.Args, "--saveto=")
		if err := os.WriteFile(filepath.Join(saveDir, "partial.download"), []byte(content), 0o600); err != nil {
			return execx.Result{}, err
		}
		ready <- struct{}{}
		<-release
		return execx.Result{ExitCode: 1}, errors.New("download interrupted")
	}}
	fs := newBaiduFileSystem(t, runner, "account")
	destination := filepath.Join(t.TempDir(), "target.mp4")
	errorsOut := make(chan error, 2)
	for range 2 {
		go func() {
			_, err := fs.(remotefs.Downloader).Download(context.Background(), remotefs.DownloadRequest{
				Reference: remotefs.Reference{Path: "/shows/episode 01.mp4"}, Destination: destination, PartialFile: remotefs.PartialFileKeep,
			}, nil)
			errorsOut <- err
		}()
	}
	for range 2 {
		select {
		case <-ready:
		case <-time.After(time.Second):
			t.Fatal("concurrent downloads did not reach transfer")
		}
	}
	close(release)
	conflicts := 0
	for range 2 {
		err := <-errorsOut
		if err == nil {
			t.Fatal("interrupted download error = nil")
		}
		if errors.Is(err, remotefs.ErrConflict) {
			conflicts++
		}
	}
	if conflicts != 1 {
		t.Fatalf("partial ownership conflicts = %d, want 1", conflicts)
	}
	data, err := os.ReadFile(destination + ".partial")
	if err != nil || (string(data) != "partial-1" && string(data) != "partial-2") {
		t.Fatalf("preserved partial = %q, %v", data, err)
	}
}

func TestBaiduPanRemoveRequiresExplicitEnablement(t *testing.T) {
	runner := &fakeBaiduRunner{}
	disabled := newBaiduFileSystem(t, runner, "disabled")
	if _, ok := disabled.(remotefs.Remover); ok {
		t.Fatal("disabled baidupan unexpectedly exposes Remove")
	}
	enabled := newBaiduFileSystem(t, runner, "enabled", func(cfg *baiduTestConfig) { cfg.session.AllowRemove = true })
	remover, ok := enabled.(remotefs.Remover)
	if !ok {
		t.Fatal("enabled baidupan does not expose Remove")
	}
	if err := remover.Remove(context.Background(), remotefs.RemoveRequest{Reference: remotefs.Reference{Path: "/shows/episode 01.mp4"}}); err != nil {
		t.Fatalf("remove: %v", err)
	}
	commands := runner.snapshot()
	for _, command := range commands {
		if commandName(command) == "rm" && command.Input != "y\n" {
			t.Fatalf("remove command input = %q", command.Input)
		}
	}
}

func TestBaiduPanConditionalRemoveRejectsChangedSource(t *testing.T) {
	tests := []remotefs.RemoveRequest{
		{Reference: remotefs.Reference{ID: "different-object", Path: "/shows/episode 01.mp4"}},
		{Reference: remotefs.Reference{Path: "/shows/episode 01.mp4"}, ExpectedVersion: "different-version"},
	}
	for index, request := range tests {
		t.Run(fmt.Sprintf("case-%d", index), func(t *testing.T) {
			runner := &fakeBaiduRunner{}
			fs := newBaiduFileSystem(t, runner, fmt.Sprintf("conditional-remove-%d", index), func(cfg *baiduTestConfig) {
				cfg.session.AllowRemove = true
			})
			err := fs.(remotefs.Remover).Remove(context.Background(), request)
			if !errors.Is(err, remotefs.ErrSourceChanged) {
				t.Fatalf("Remove() error = %v, want ErrSourceChanged", err)
			}
			for _, command := range runner.snapshot() {
				if commandName(command) == "rm" {
					t.Fatalf("Remove() executed provider rm after snapshot mismatch: %+v", command)
				}
			}
		})
	}
}

func TestBaiduPanDriverContract(t *testing.T) {
	runner := &fakeBaiduRunner{}
	runner.streamFn = func(ctx context.Context, command baidupan.Command, sink execx.Sink) (execx.Result, error) {
		saveDir := commandArgValue(command.Args, "--saveto=")
		finalPath := filepath.Join(saveDir, "episode 01.mp4")
		if err := os.WriteFile(finalPath, []byte("0123456789"), 0o600); err != nil {
			return execx.Result{}, err
		}
		line := "[1] 下载完成, 保存位置: " + finalPath
		if err := sink.WriteCommandEvent(ctx, execx.Event{Text: line, Data: []byte(line)}); err != nil {
			return execx.Result{}, err
		}
		return execx.Result{ExitCode: 0}, nil
	}
	fs := newBaiduFileSystem(t, runner, "contract-account")
	testsuite.Run(t, testsuite.Fixture{
		FileSystem: fs,
		Root:       remotefs.Reference{Path: "/shows"},
		File:       remotefs.Reference{Path: "/shows/episode 01.mp4"},
		Data:       []byte("0123456789"),
	})
}

func envValue(env []string, key string) string {
	prefix := key + "="
	for _, value := range env {
		if strings.HasPrefix(value, prefix) {
			return strings.TrimPrefix(value, prefix)
		}
	}
	return ""
}

func commandArgValue(args []string, prefix string) string {
	for _, arg := range args {
		if strings.HasPrefix(arg, prefix) {
			return strings.TrimPrefix(arg, prefix)
		}
	}
	return ""
}

func countCommands(commands []baidupan.Command, name string) int {
	count := 0
	for _, command := range commands {
		if commandName(command) == name {
			count++
		}
	}
	return count
}

func statefulCommands(commands []baidupan.Command) []string {
	var result []string
	for _, command := range commands {
		switch commandName(command) {
		case "mkdir":
			result = append(result, "mkdir:"+command.Args[len(command.Args)-1])
		case "cd":
			result = append(result, "cd:"+command.Args[len(command.Args)-1])
		case "transfer":
			result = append(result, "transfer")
		}
	}
	return result
}
