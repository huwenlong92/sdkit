package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type dailyWriterConfig struct {
	Dir    string
	Name   string
	Ext    string
	MaxAge int
	Now    func() time.Time
}

type dailyWriter struct {
	mu      sync.Mutex
	cfg     dailyWriterConfig
	current string
	file    *os.File
}

func newDailyWriter(cfg dailyWriterConfig) *dailyWriter {
	if cfg.Ext == "" {
		cfg.Ext = ".log"
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &dailyWriter{cfg: cfg}
}

func (w *dailyWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	day := w.cfg.Now().Format("2006-01-02")
	if err := w.rotate(day); err != nil {
		return 0, err
	}
	return w.file.Write(p)
}

func (w *dailyWriter) rotate(day string) error {
	if w.file != nil && w.current == day {
		return nil
	}
	if w.file != nil {
		_ = w.file.Close()
		w.file = nil
	}
	if err := os.MkdirAll(w.cfg.Dir, 0755); err != nil {
		return err
	}
	file, err := os.OpenFile(w.path(day), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	w.file = file
	w.current = day
	w.cleanup(day)
	return nil
}

func (w *dailyWriter) path(day string) string {
	return filepath.Join(w.cfg.Dir, fmt.Sprintf("%s-%s%s", w.cfg.Name, day, w.cfg.Ext))
}

func (w *dailyWriter) cleanup(today string) {
	if w.cfg.MaxAge <= 0 {
		return
	}
	cutoff := w.cfg.Now().AddDate(0, 0, -w.cfg.MaxAge)
	entries, err := os.ReadDir(w.cfg.Dir)
	if err != nil {
		return
	}
	prefix := w.cfg.Name + "-"
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		day, ok := w.logDay(name, prefix)
		if !ok || day == today {
			continue
		}
		t, err := time.Parse("2006-01-02", day)
		if err != nil || !t.Before(cutoff) {
			continue
		}
		_ = os.Remove(filepath.Join(w.cfg.Dir, name))
	}
}

func (w *dailyWriter) logDay(filename string, prefix string) (string, bool) {
	if !strings.HasPrefix(filename, prefix) {
		return "", false
	}
	filename = strings.TrimPrefix(filename, prefix)
	filename = strings.TrimSuffix(filename, ".gz")
	if !strings.HasSuffix(filename, w.cfg.Ext) {
		return "", false
	}
	day := strings.TrimSuffix(filename, w.cfg.Ext)
	if len(day) != len("2006-01-02") {
		return "", false
	}
	return day, true
}

func (w *dailyWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	return err
}
