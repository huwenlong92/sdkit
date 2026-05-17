package tests

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/huwenlong92/sdkit/core/config"
	"github.com/huwenlong92/sdkit/core/logger"
	fscore "github.com/huwenlong92/sdkit/pkg/filesystem/core"
)

func TestFilesystemConfigKeepsDriverConfig(t *testing.T) {
	if err := logger.Configure(logger.Config{Name: "config-test", Level: "debug", Mode: "dev"}); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := []byte(`
app:
  name: test
filesystem:
  driver: local
  token_ttl: 2h
  local:
    dir: storage
    public_url: https://static.example.com/files
`)
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}

	var cfg struct {
		FileSystem fscore.Config `mapstructure:"filesystem"`
	}
	err := config.Load(path, &cfg)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.FileSystem.Driver != "local" {
		t.Fatalf("driver = %q", cfg.FileSystem.Driver)
	}
	if cfg.FileSystem.TokenTTL != 2*time.Hour {
		t.Fatalf("token ttl = %s", cfg.FileSystem.TokenTTL)
	}
	if got := cfg.FileSystem.DriverString("local", "dir"); got != "storage" {
		t.Fatalf("local dir = %q", got)
	}
	if got := cfg.FileSystem.DriverString("local", "public_url"); got != "https://static.example.com/files" {
		t.Fatalf("local public_url = %q", got)
	}
}

func TestConfigImportsMergeFeatureFiles(t *testing.T) {
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "config.yaml")
	mainContent := []byte(`
imports:
  - services.yaml
  - worker.yaml
app:
  name: test
worker:
  queue:
    concurrency: 2
`)
	if err := os.WriteFile(mainPath, mainContent, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "services.yaml"), []byte(`
services:
  api:
    type: api
    enabled: true
    addr: :8081
`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "worker.yaml"), []byte(`
worker:
  queue:
    addr: 127.0.0.1:6379
    concurrency: 10
`), 0644); err != nil {
		t.Fatal(err)
	}

	var cfg struct {
		App struct {
			Name string `mapstructure:"name"`
		} `mapstructure:"app"`
		Services map[string]struct {
			Type string `mapstructure:"type"`
			Addr string `mapstructure:"addr"`
		} `mapstructure:"services"`
		Worker struct {
			Queue struct {
				Addr        string `mapstructure:"addr"`
				Concurrency int    `mapstructure:"concurrency"`
			} `mapstructure:"queue"`
		} `mapstructure:"worker"`
	}
	if err := config.Load(mainPath, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.App.Name != "test" {
		t.Fatalf("app name = %q", cfg.App.Name)
	}
	if cfg.Services["api"].Addr != ":8081" {
		t.Fatalf("api addr = %q", cfg.Services["api"].Addr)
	}
	if cfg.Worker.Queue.Addr != "127.0.0.1:6379" {
		t.Fatalf("queue addr = %q", cfg.Worker.Queue.Addr)
	}
	if cfg.Worker.Queue.Concurrency != 10 {
		t.Fatalf("queue concurrency = %d", cfg.Worker.Queue.Concurrency)
	}
}

func TestLoadRequiredKeyFailsWhenKeyMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(`
app:
  name: test
`), 0644); err != nil {
		t.Fatal(err)
	}

	var out struct {
		Driver string `mapstructure:"driver"`
	}
	if err := config.LoadRequiredKey(path, "eventbus", &out); err == nil {
		t.Fatal("expected missing key error")
	}
}

func TestLoadRequiredKeyReadsExistingKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(`
eventbus:
  driver: memory
`), 0644); err != nil {
		t.Fatal(err)
	}

	var out struct {
		Driver string `mapstructure:"driver"`
	}
	if err := config.LoadRequiredKey(path, "eventbus", &out); err != nil {
		t.Fatal(err)
	}
	if out.Driver != "memory" {
		t.Fatalf("driver = %q", out.Driver)
	}
}
