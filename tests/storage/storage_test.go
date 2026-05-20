package storage_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	coreconfig "github.com/huwenlong92/sdkit/core/config"
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
				UseSSL:    false,
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

func TestFacadeLoadsStorageConfigFromCoreConfig(t *testing.T) {
	t.Cleanup(func() {
		_ = corestorage.Close()
		coreconfig.V = nil
	})

	defaultDir := t.TempDir()
	archiveDir := t.TempDir()
	configPath := writeConfig(t, `
storage:
  default: primary
  stores:
    primary:
      driver: local
      local_dir: `+defaultDir+`
    archive:
      driver: local
      local_dir: `+archiveDir+`
`)
	if _, err := coreconfig.New(configPath); err != nil {
		t.Fatalf("load config: %v", err)
	}

	app := runtime.New()
	capability := storagecap.Use()
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

func TestFacadeUsesDefaultLocalConfig(t *testing.T) {
	t.Cleanup(func() {
		_ = corestorage.Close()
		coreconfig.V = nil
	})
	coreconfig.V = nil

	app := runtime.New()
	capability := storagecap.Use()
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

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(strings.TrimSpace(content)+"\n"), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
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
