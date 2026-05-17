package logger

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"gopkg.in/natefinch/lumberjack.v2"
)

func TestNormalizeConfigDefaultsToSizeRotation(t *testing.T) {
	cfg := normalizeConfig(Config{})

	if cfg.Rotation.Mode != "size" {
		t.Fatalf("rotation mode = %q, want size", cfg.Rotation.Mode)
	}
	if cfg.Rotation.MaxSize != defaultMaxSize {
		t.Fatalf("max size = %d, want %d", cfg.Rotation.MaxSize, defaultMaxSize)
	}
}

func TestWriterUsesDailyRotationMode(t *testing.T) {
	restoreLoggerConfig(t)
	global = Config{
		RootDir: t.TempDir(),
		Rotation: RotationConfig{
			Mode:   "daily",
			MaxAge: 30,
		},
	}

	writer, err := Writer("api", "api.log")
	if err != nil {
		t.Fatalf("Writer() error = %v", err)
	}
	if _, ok := writer.(*dailyWriter); !ok {
		t.Fatalf("Writer() = %T, want *dailyWriter", writer)
	}
}

func TestDailyWriterUsesFilenameBase(t *testing.T) {
	restoreLoggerConfig(t)
	global = Config{
		RootDir: t.TempDir(),
		Rotation: RotationConfig{
			Mode:   "daily",
			MaxAge: 30,
		},
	}

	writer, err := Writer("gorm", "runtime.log")
	if err != nil {
		t.Fatalf("Writer() error = %v", err)
	}
	daily, ok := writer.(*dailyWriter)
	if !ok {
		t.Fatalf("Writer() = %T, want *dailyWriter", writer)
	}
	if daily.cfg.Name != "runtime" {
		t.Fatalf("daily writer name = %q, want runtime", daily.cfg.Name)
	}
}

func TestWriterUsesSizeRotationByDefault(t *testing.T) {
	restoreLoggerConfig(t)
	global = Config{RootDir: t.TempDir()}

	writer, err := Writer("api", "api.log")
	if err != nil {
		t.Fatalf("Writer() error = %v", err)
	}
	if _, ok := writer.(*lumberjack.Logger); !ok {
		t.Fatalf("Writer() = %T, want *lumberjack.Logger", writer)
	}
}

func TestDailyWriterRotatesByDate(t *testing.T) {
	now := time.Date(2026, 5, 17, 10, 0, 0, 0, time.UTC)
	writer := newDailyWriter(dailyWriterConfig{
		Dir:    t.TempDir(),
		Name:   "api",
		Ext:    ".log",
		MaxAge: 30,
		Now: func() time.Time {
			return now
		},
	})

	if _, err := writer.Write([]byte("first\n")); err != nil {
		t.Fatalf("Write(first) error = %v", err)
	}
	now = now.AddDate(0, 0, 1)
	if _, err := writer.Write([]byte("second\n")); err != nil {
		t.Fatalf("Write(second) error = %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	assertFileContains(t, filepath.Join(writer.cfg.Dir, "api-2026-05-17.log"), "first\n")
	assertFileContains(t, filepath.Join(writer.cfg.Dir, "api-2026-05-18.log"), "second\n")
}

func restoreLoggerConfig(t *testing.T) {
	t.Helper()
	prev := global
	t.Cleanup(func() {
		global = prev
	})
}

func assertFileContains(t *testing.T, path string, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	if string(data) != want {
		t.Fatalf("file %s = %q, want %q", path, string(data), want)
	}
}
