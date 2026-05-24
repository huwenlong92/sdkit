package sandbox

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func validateFiles(files []File, opts Options) error {
	if len(files) > opts.MaxFiles {
		return fmt.Errorf("%w: too many files: %d > %d", ErrInvalidRequest, len(files), opts.MaxFiles)
	}
	var total int64
	seen := map[string]struct{}{}
	for _, file := range files {
		clean, err := cleanFilePath(file.Path)
		if err != nil {
			return err
		}
		if _, ok := seen[clean]; ok {
			return fmt.Errorf("%w: duplicate file path %q", ErrInvalidRequest, clean)
		}
		seen[clean] = struct{}{}
		size := int64(len(file.Content))
		if size > opts.MaxFileBytes {
			return fmt.Errorf("%w: file %q too large: %d > %d", ErrInvalidRequest, clean, size, opts.MaxFileBytes)
		}
		total += size
		if total > opts.MaxTotalFileSize {
			return fmt.Errorf("%w: total file size too large: %d > %d", ErrInvalidRequest, total, opts.MaxTotalFileSize)
		}
	}
	return nil
}

func WriteFiles(workspace string, files []File) error {
	if workspace == "" {
		return fmt.Errorf("%w: workspace required", ErrInvalidRequest)
	}
	for _, file := range files {
		rel, err := cleanFilePath(file.Path)
		if err != nil {
			return err
		}
		target := filepath.Join(workspace, rel)
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return fmt.Errorf("sandbox write file mkdir %q: %w", rel, err)
		}
		mode := os.FileMode(file.Mode)
		if mode == 0 {
			mode = 0644
		}
		if err := os.WriteFile(target, file.Content, mode); err != nil {
			return fmt.Errorf("sandbox write file %q: %w", rel, err)
		}
	}
	return nil
}

func cleanFilePath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("%w: empty file path", ErrInvalidRequest)
	}
	if filepath.IsAbs(path) {
		return "", fmt.Errorf("%w: absolute file path %q", ErrInvalidRequest, path)
	}
	clean := filepath.Clean(path)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: unsafe file path %q", ErrInvalidRequest, path)
	}
	return clean, nil
}
