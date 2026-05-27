package storage_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/huwenlong92/sdkit/core/runtime"
	corestorage "github.com/huwenlong92/sdkit/core/storage"
	storagecap "github.com/huwenlong92/sdkit/core/storage/facade"
)

func TestManagerDefaultAndNamedStores(t *testing.T) {
	t.Cleanup(func() {
		_ = corestorage.Close()
	})

	defaultDir := t.TempDir()
	archiveDir := t.TempDir()
	manager, err := corestorage.NewManager(corestorage.Config{
		Default: "primary",
		Stores: map[string]corestorage.StoreConfig{
			"primary": {
				Driver:   "local",
				LocalDir: defaultDir,
			},
			"archive": {
				Driver:   "local",
				LocalDir: archiveDir,
			},
			"minio": {
				Driver:    "minio",
				Bucket:    "assets",
				Endpoint:  "http://127.0.0.1:9000",
				AccessKey: "minio",
				SecretKey: "minio-secret",
			},
		},
	})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	corestorage.SetDefault(manager)

	defaultFS, err := manager.Default()
	if err != nil {
		t.Fatalf("default store: %v", err)
	}
	archiveFS, err := manager.Use("archive")
	if err != nil {
		t.Fatalf("archive store: %v", err)
	}
	var hookCalled bool
	defaultResult := defaultFS.UploadStreamWithHook(context.Background(), strings.NewReader("default"), corestorage.FileInfo{Name: "a.txt", Size: 7},
		corestorage.BeforeUpload(func(ctx context.Context, event corestorage.Event) error {
			hookCalled = event.File.Name == "a.txt"
			return nil
		}),
	)
	if defaultResult.Error != nil {
		t.Fatalf("upload default: %v", defaultResult.Error)
	}
	if !hookCalled {
		t.Fatal("upload hook was not called")
	}
	archiveResult := archiveFS.UploadStream(context.Background(), strings.NewReader("archive"), corestorage.FileInfo{Name: "b.txt", Size: 7})
	if archiveResult.Error != nil {
		t.Fatalf("upload archive: %v", archiveResult.Error)
	}

	assertFileContent(t, filepath.Join(defaultDir, defaultResult.File.Path), "default")
	assertFileContent(t, filepath.Join(archiveDir, archiveResult.File.Path), "archive")

	policy, err := manager.Policy("minio")
	if err != nil {
		t.Fatalf("minio policy: %v", err)
	}
	if policy.Driver != "minio" || policy.Bucket != "assets" || policy.Endpoint != "http://127.0.0.1:9000" {
		t.Fatalf("unexpected minio policy: %+v", policy)
	}
}

func TestManagerRequiresDefaultStore(t *testing.T) {
	_, err := corestorage.NewManager(corestorage.Config{
		Stores: map[string]corestorage.StoreConfig{
			"local": {
				Driver:   "local",
				LocalDir: t.TempDir(),
			},
		},
	})
	if !errors.Is(err, corestorage.ErrDefaultRequired) {
		t.Fatalf("expected ErrDefaultRequired, got %v", err)
	}
}

func TestRemoteDriverRequiresRegistration(t *testing.T) {
	_, err := corestorage.New(corestorage.Policy{
		Driver: "oss",
	})
	if !errors.Is(err, corestorage.ErrUnknownDriver) {
		t.Fatalf("expected ErrUnknownDriver, got %v", err)
	}
}

func TestFacadeLoadsStorageConfigFromConfigLoader(t *testing.T) {
	t.Cleanup(func() {
		_ = corestorage.Close()
	})

	defaultDir := t.TempDir()
	archiveDir := t.TempDir()
	cfg := storagecap.Config{
		Default: "primary",
		Stores: map[string]storagecap.StoreConfig{
			"primary": {
				Driver:   "local",
				LocalDir: defaultDir,
			},
			"archive": {
				Driver:   "local",
				LocalDir: archiveDir,
			},
		},
	}

	app := runtime.New()
	capability := storagecap.Use(storagecap.WithConfigLoader(func(*runtime.App) (storagecap.Config, error) {
		return cfg, nil
	}))
	if err := capability.Register(app); err != nil {
		t.Fatalf("register storage: %v", err)
	}
	t.Cleanup(func() {
		if err := capability.Shutdown(context.Background()); err != nil {
			t.Fatalf("shutdown storage: %v", err)
		}
	})

	manager := storagecap.From(app)
	if manager == nil {
		t.Fatal("storage manager not bound")
	}
	if manager.DefaultName() != "primary" {
		t.Fatalf("unexpected default name: %s", manager.DefaultName())
	}

	archiveFS, err := corestorage.Use("archive")
	if err != nil {
		t.Fatalf("use archive: %v", err)
	}
	result := archiveFS.UploadStream(context.Background(), strings.NewReader("archive"), corestorage.FileInfo{Name: "c.txt", Size: 7})
	if result.Error != nil {
		t.Fatalf("upload archive: %v", result.Error)
	}
	assertFileContent(t, filepath.Join(archiveDir, result.File.Path), "archive")
}

func TestFacadeUsesExplicitDefaultLocalConfig(t *testing.T) {
	t.Cleanup(func() {
		_ = corestorage.Close()
	})

	app := runtime.New()
	capability := storagecap.Use(storagecap.WithConfig(storagecap.DefaultConfig()))
	if err := capability.Register(app); err != nil {
		t.Fatalf("register storage: %v", err)
	}
	t.Cleanup(func() {
		if err := capability.Shutdown(context.Background()); err != nil {
			t.Fatalf("shutdown storage: %v", err)
		}
	})

	manager := storagecap.From(app)
	if manager == nil {
		t.Fatal("storage manager not bound")
	}
	if manager.DefaultName() != corestorage.DefaultStoreName {
		t.Fatalf("default name = %q, want %q", manager.DefaultName(), corestorage.DefaultStoreName)
	}
	policy, err := manager.Policy("")
	if err != nil {
		t.Fatalf("default policy: %v", err)
	}
	if policy.Driver != "local" || policy.LocalDir != "storage" {
		t.Fatalf("unexpected default policy: %+v", policy)
	}
}

func TestLocalSourceBuildsSignedPrivateURL(t *testing.T) {
	dir := t.TempDir()
	fs, err := corestorage.New(corestorage.Policy{
		Driver:    "local",
		LocalDir:  dir,
		Endpoint:  "https://files.example.com/private/source",
		SecretKey: "test-source-secret",
	})
	if err != nil {
		t.Fatalf("new storage: %v", err)
	}

	result := fs.UploadStream(context.Background(), strings.NewReader("private file"), corestorage.FileInfo{
		Name: "report.txt",
		Path: "private/report.txt",
		Size: int64(len("private file")),
	})
	if result.Error != nil {
		t.Fatalf("upload: %v", result.Error)
	}

	source, err := fs.Source(result.File.Path, time.Minute)
	if err != nil {
		t.Fatalf("source: %v", err)
	}
	parsed, err := url.Parse(source)
	if err != nil {
		t.Fatalf("parse source: %v", err)
	}
	if parsed.Scheme != "https" || parsed.Host != "files.example.com" || parsed.Path != "/private/source" {
		t.Fatalf("unexpected source base: %s", source)
	}
	query := parsed.Query()
	if query.Get("path") != result.File.Path || query.Get("expires") == "" || query.Get("signature") == "" {
		t.Fatalf("source query missing signed fields: %s", source)
	}

	req := httptest.NewRequest(http.MethodGet, source, nil)
	resp := httptest.NewRecorder()
	corestorage.SourceHandler(fs, "test-source-secret").ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("source handler status = %d body=%s", resp.Code, resp.Body.String())
	}
	if disposition := resp.Header().Get("Content-Disposition"); !strings.HasPrefix(disposition, "inline") {
		t.Fatalf("content disposition = %q, want inline", disposition)
	}
	if contentType := resp.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "text/plain") {
		t.Fatalf("content type = %q, want text/plain", contentType)
	}
	body, err := io.ReadAll(resp.Result().Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	if string(body) != "private file" {
		t.Fatalf("source handler body = %q", string(body))
	}

	query.Set("download", "1")
	parsed.RawQuery = query.Encode()
	req = httptest.NewRequest(http.MethodGet, parsed.String(), nil)
	resp = httptest.NewRecorder()
	corestorage.SourceHandler(fs, "test-source-secret").ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("download source handler status = %d body=%s", resp.Code, resp.Body.String())
	}
	if disposition := resp.Header().Get("Content-Disposition"); !strings.HasPrefix(disposition, "attachment") {
		t.Fatalf("download content disposition = %q, want attachment", disposition)
	}
}

func TestLocalSourceRejectsInvalidSignature(t *testing.T) {
	fs, err := corestorage.New(corestorage.Policy{
		Driver:    "local",
		LocalDir:  t.TempDir(),
		SecretKey: "test-source-secret",
	})
	if err != nil {
		t.Fatalf("new storage: %v", err)
	}
	source, err := fs.Source("private/report.txt", time.Minute)
	if err != nil {
		t.Fatalf("source: %v", err)
	}
	parsed, err := url.Parse(source)
	if err != nil {
		t.Fatalf("parse source: %v", err)
	}
	query := parsed.Query()
	query.Set("signature", "bad")
	parsed.RawQuery = query.Encode()

	req := httptest.NewRequest(http.MethodGet, parsed.String(), nil)
	resp := httptest.NewRecorder()
	corestorage.SourceHandler(fs, "test-source-secret").ServeHTTP(resp, req)
	if resp.Code != http.StatusForbidden {
		t.Fatalf("source handler status = %d, want %d", resp.Code, http.StatusForbidden)
	}
}

func assertFileContent(t *testing.T, path string, expected string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(content) != expected {
		t.Fatalf("unexpected %s content: %q", path, string(content))
	}
}
