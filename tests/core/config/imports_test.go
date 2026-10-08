package tests

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/huwenlong92/sdkit/core/config"
	"github.com/spf13/afero"
	"github.com/spf13/viper"
)

func writeImportFixture(t *testing.T, directory string, name string, contents string) string {
	t.Helper()
	path := filepath.Join(directory, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestConfigImportsResolveEachFileDirectory(t *testing.T) {
	directory := t.TempDir()
	root := writeImportFixture(t, directory, "config.yaml", "imports:\n  - ' features/files.yaml '\n  - ' '\napp:\n  name: example\n")
	writeImportFixture(t, directory, "features/files.yaml", "imports:\n  - drivers/cloud.yaml\nfiles:\n  enabled: true\n")
	writeImportFixture(t, directory, "features/drivers/cloud.yaml", "cloud:\n  binary_path: bin/example-tool\n")
	// The entrypoint directory contains an unrelated file with the same basename.
	writeImportFixture(t, directory, "drivers/cloud.yaml", "cloud:\n  binary_path: wrong-path\n")
	merged, err := config.New(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := merged.GetString("cloud.binary_path"); got != "bin/example-tool" {
		t.Fatalf("cloud binary = %q, want bin/example-tool", got)
	}
	if !merged.GetBool("files.enabled") || merged.GetString("app.name") != "example" {
		t.Fatal("parent configuration was not retained")
	}
	if got := merged.ConfigFileUsed(); got != root {
		t.Fatalf("ConfigFileUsed = %q, want %q", got, root)
	}
	var out struct {
		BinaryPath string `mapstructure:"binary_path"`
	}
	if err := config.LoadRequiredKey(root, "cloud", &out); err != nil {
		t.Fatal(err)
	}
	if out.BinaryPath != "bin/example-tool" {
		t.Fatalf("LoadRequiredKey binary = %q", out.BinaryPath)
	}
}

func TestConfigImportsPreserveMergeOrder(t *testing.T) {
	cases := []struct {
		name          string
		firstImports  string
		secondImports string
		want          string
	}{
		{name: "flat", want: "second"},
		{name: "nested_then_sibling", firstImports: "imports:\n  - shared.yaml\n", want: "second"},
		{name: "shared_file_in_two_branches", firstImports: "imports:\n  - shared.yaml\n", secondImports: "imports:\n  - shared.yaml\n", want: "shared"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			directory := t.TempDir()
			root := writeImportFixture(t, directory, "config.yaml", "imports:\n  - first.yaml\n  - second.yaml\nsetting:\n  value: root\n  preserved: true\n")
			writeImportFixture(t, directory, "first.yaml", tc.firstImports+"setting:\n  value: first\n")
			writeImportFixture(t, directory, "second.yaml", tc.secondImports+"setting:\n  value: second\n")
			writeImportFixture(t, directory, "shared.yaml", "setting:\n  value: shared\n")
			merged, err := config.New(root)
			if err != nil {
				t.Fatal(err)
			}
			if got := merged.GetString("setting.value"); got != tc.want {
				t.Fatalf("merged value = %q, want %q", got, tc.want)
			}
			if !merged.GetBool("setting.preserved") {
				t.Fatal("nested map merge dropped a parent key")
			}
		})
	}
}

func TestConfigImportsSupportAbsolutePathsAndDefaults(t *testing.T) {
	directory := t.TempDir()
	child := writeImportFixture(t, t.TempDir(), "child.yaml", "setting:\n  value: child\n")
	root := writeImportFixture(t, directory, "config.yaml", fmt.Sprintf("imports:\n  - %q\nsetting:\n  explicit: parent\n", child))
	merged, err := config.New(root, config.WithDefaults(map[string]any{
		"setting.value":    "default",
		"setting.explicit": "default",
		"setting.fallback": "fallback",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if got := merged.GetString("setting.value"); got != "child" {
		t.Fatalf("value = %q, want child", got)
	}
	if got := merged.GetString("setting.explicit"); got != "parent" {
		t.Fatalf("explicit = %q, want parent", got)
	}
	if got := merged.GetString("setting.fallback"); got != "fallback" {
		t.Fatalf("fallback = %q", got)
	}
	t.Setenv("SDKITGO_SETTING_VALUE", "environment")
	merged, err = config.New(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := merged.GetString("setting.value"); got != "environment" {
		t.Fatalf("environment override = %q", got)
	}
}

func TestConfigImportsDetectCycles(t *testing.T) {
	cases := []struct {
		name    string
		entry   string
		child   string
		symlink bool
	}{
		{name: "self", entry: "./config.yaml"},
		{name: "indirect_with_parent_path", entry: "nested/child.yaml", child: "../config.yaml"},
		{name: "symlink_alias", entry: "alias.yaml", symlink: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			directory := t.TempDir()
			root := writeImportFixture(t, directory, "config.yaml", fmt.Sprintf("imports:\n  - %s\n", tc.entry))
			if tc.child != "" {
				writeImportFixture(t, directory, "nested/child.yaml", fmt.Sprintf("imports:\n  - %s\n", tc.child))
			}
			if tc.symlink {
				if err := os.Symlink(root, filepath.Join(directory, "alias.yaml")); err != nil {
					t.Fatal(err)
				}
			}
			previous := config.V
			merged, err := config.New(root)
			if err == nil || !strings.Contains(err.Error(), "config import cycle:") || !strings.Contains(err.Error(), root) || !strings.Contains(err.Error(), " -> ") {
				t.Fatalf("cycle error = %v", err)
			}
			if tc.child != "" && !strings.Contains(err.Error(), "nested/child.yaml") {
				t.Fatalf("cycle error lacks intermediate file: %v", err)
			}
			if merged != nil || config.V != previous {
				t.Fatal("failed load replaced the usable config")
			}
		})
	}
}

func TestConfigImportsErrorsKeepFullChainAndCause(t *testing.T) {
	cases := []struct {
		name    string
		invalid bool
	}{
		{name: "missing_file"},
		{name: "invalid_yaml", invalid: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			directory := t.TempDir()
			root := writeImportFixture(t, directory, "config.yaml", "imports:\n  - nested/child.yaml\n")
			child := writeImportFixture(t, directory, "nested/child.yaml", "imports:\n  - leaf.yaml\n")
			leaf := filepath.Join(directory, "nested/leaf.yaml")
			if tc.invalid {
				writeImportFixture(t, directory, "nested/leaf.yaml", "broken: [\n")
			}
			previous := config.V
			_, err := config.New(root)
			if err == nil {
				t.Fatal("expected import error")
			}
			chain := root + " -> " + child + " -> " + leaf
			if !strings.Contains(err.Error(), chain) {
				t.Fatalf("error = %v, want chain %q", err, chain)
			}
			if tc.invalid {
				var parseError viper.ConfigParseError
				if !errors.As(err, &parseError) {
					t.Fatalf("error = %T, want wrapped ConfigParseError", err)
				}
			} else if !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("error = %v, want missing-file cause", err)
			}
			if config.V != previous {
				t.Fatal("failed import replaced config.V")
			}
		})
	}
}

func TestConfigImportsKeepCustomFilesystem(t *testing.T) {
	fs := afero.NewMemMapFs()
	root := "/virtual/config.yaml"
	child := "/virtual/nested/child.yaml"
	leaf := "/virtual/nested/leaf.yaml"
	if err := fs.MkdirAll("/virtual/nested", 0o700); err != nil {
		t.Fatal(err)
	}
	files := []struct {
		path     string
		contents string
	}{
		{path: root, contents: "imports:\n  - nested/child.yaml\n"},
		{path: child, contents: "imports:\n  - leaf.yaml\n"},
		{path: leaf, contents: "setting:\n  value: virtual\n"},
	}
	for _, file := range files {
		if err := afero.WriteFile(fs, file.path, []byte(file.contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	merged, err := config.New(root, func(v *viper.Viper) { v.SetFs(fs) })
	if err != nil {
		t.Fatal(err)
	}
	if got := merged.GetString("setting.value"); got != "virtual" {
		t.Fatalf("virtual value = %q, want virtual", got)
	}
	if got := merged.ConfigFileUsed(); got != root {
		t.Fatalf("ConfigFileUsed = %q, want %q", got, root)
	}
}
