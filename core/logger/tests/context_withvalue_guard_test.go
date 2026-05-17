package tests

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

var allowedContextWithValueExceptions = map[string]bool{}

func TestNoDirectContextWithValueForStandardLoggerFields(t *testing.T) {
	root := repoRoot(t)
	fset := token.NewFileSet()

	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
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

		file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			return err
		}
		contextNames := contextImportNames(file)
		if len(contextNames) == 0 {
			return nil
		}
		packageNames := standardPackageNames(file)
		rel, err := filepath.Rel(root, path)
		if err != nil {
			rel = path
		}

		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			funcName := fn.Name.Name
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok || !isContextWithValue(call, contextNames) || len(call.Args) < 2 {
					return true
				}
				key, ok := standardContextFieldKey(call.Args[1], file.Name.Name, packageNames)
				if !ok {
					return true
				}
				if allowedContextWithValueExceptions[rel+":"+funcName] {
					return true
				}
				pos := fset.Position(call.Pos())
				t.Errorf("%s:%d uses context.WithValue with standard field %s; use tracking.WithTrackID, requestid.WithRequestID, or logger.WithField", rel, pos.Line, key)
				return true
			})
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan context.WithValue: %v", err)
	}
}

func contextImportNames(file *ast.File) map[string]bool {
	names := map[string]bool{}
	for _, imp := range file.Imports {
		if strings.Trim(imp.Path.Value, `"`) != "context" {
			continue
		}
		name := "context"
		if imp.Name != nil {
			name = imp.Name.Name
		}
		if name != "." && name != "_" {
			names[name] = true
		}
	}
	return names
}

func standardPackageNames(file *ast.File) map[string]string {
	paths := map[string]string{
		"github.com/huwenlong92/sdkit/core/logger":    "logger",
		"github.com/huwenlong92/sdkit/core/requestid": "requestid",
		"github.com/huwenlong92/sdkit/core/tracking":  "tracking",
	}
	names := map[string]string{}
	for _, imp := range file.Imports {
		importPath := strings.Trim(imp.Path.Value, `"`)
		defaultName, ok := paths[importPath]
		if !ok {
			continue
		}
		name := defaultName
		if imp.Name != nil {
			name = imp.Name.Name
		}
		if name != "." && name != "_" {
			names[name] = defaultName
		}
	}
	return names
}

func isContextWithValue(call *ast.CallExpr, contextNames map[string]bool) bool {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "WithValue" {
		return false
	}
	ident, ok := selector.X.(*ast.Ident)
	return ok && contextNames[ident.Name]
}

func standardContextFieldKey(expr ast.Expr, packageName string, packageAliases map[string]string) (string, bool) {
	switch key := expr.(type) {
	case *ast.SelectorExpr:
		pkg, ok := key.X.(*ast.Ident)
		if !ok {
			return "", false
		}
		switch packageAliases[pkg.Name] {
		case "logger":
			if isLoggerFieldKey(key.Sel.Name) {
				return "logger." + key.Sel.Name, true
			}
		case "requestid":
			if key.Sel.Name == "Key" {
				return "requestid.Key", true
			}
		case "tracking":
			if key.Sel.Name == "Key" {
				return "tracking.Key", true
			}
		}
	case *ast.Ident:
		if packageName == "logger" && isLoggerFieldKey(key.Name) {
			return key.Name, true
		}
		if (packageName == "requestid" || packageName == "tracking") && key.Name == "Key" {
			return packageName + ".Key", true
		}
	case *ast.BasicLit:
		if key.Kind != token.STRING {
			return "", false
		}
		value, err := strconv.Unquote(key.Value)
		if err != nil {
			return "", false
		}
		if isStandardFieldName(value) {
			return strconv.Quote(value), true
		}
	}
	return "", false
}

func isLoggerFieldKey(name string) bool {
	switch name {
	case "TrackIDKey", "TraceIDKey", "SpanIDKey", "RequestIDKey",
		"TaskIDKey", "QueueKey", "TypeKey", "RunIDKey", "JobIDKey":
		return true
	default:
		return false
	}
}

func isStandardFieldName(name string) bool {
	switch name {
	case "track_id", "trace_id", "span_id", "request_id",
		"task_id", "queue", "type", "run_id", "job_id":
		return true
	default:
		return false
	}
}

func repoRoot(t *testing.T) string {
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
