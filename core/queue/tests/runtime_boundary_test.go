package tests

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestQueueRuntimeBoundaryHasNoTransportImports(t *testing.T) {
	root := queueRepoRoot(t)
	fset := token.NewFileSet()
	blocked := map[string]bool{
		"github.com/gin-gonic/gin":                               true,
		"github.com/spf13/cobra":                                 true,
		"github.com/huwenlong92/sdkit/pkg/queue/transport/gin":   true,
		"github.com/huwenlong92/sdkit/pkg/queue/transport/cobra": true,
	}

	for _, dir := range []string{"core/queue", "pkg/queue"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				switch entry.Name() {
				case ".git", ".cache", "vendor", "node_modules", "logs", "tmp":
					return filepath.SkipDir
				default:
					return nil
				}
			}
			if filepath.Ext(path) != ".go" {
				return nil
			}

			file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
			if err != nil {
				return err
			}
			for _, imp := range file.Imports {
				importPath := strings.Trim(imp.Path.Value, `"`)
				if !blocked[importPath] {
					continue
				}
				rel, err := filepath.Rel(root, path)
				if err != nil {
					rel = path
				}
				t.Errorf("%s imports transport dependency %s", rel, importPath)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("scan %s imports: %v", dir, err)
		}
	}
}

func TestQueueTransportPackagesAreRemoved(t *testing.T) {
	root := queueRepoRoot(t)
	transportRoot := filepath.Join(root, "pkg/queue/transport")
	if _, err := os.Stat(transportRoot); os.IsNotExist(err) {
		return
	} else if err != nil {
		t.Fatalf("stat transport root: %v", err)
	}

	err := filepath.WalkDir(transportRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			rel = path
		}
		t.Errorf("queue transport Go package should be removed, found %s", rel)
		return nil
	})
	if err != nil {
		t.Fatalf("scan transport root: %v", err)
	}
}

func queueRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}
