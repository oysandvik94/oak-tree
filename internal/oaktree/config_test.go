package oaktree

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestConfigFilePathUsesXDGConfigHome(t *testing.T) {
	xdg := filepath.Join(t.TempDir(), "config-home")
	t.Setenv("XDG_CONFIG_HOME", xdg)

	got, err := configFilePath()
	if err != nil {
		t.Fatal(err)
	}

	want := filepath.Join(xdg, "oak-tree", "config.toml")
	if got != want {
		t.Fatalf("configFilePath() = %q, want %q", got, want)
	}
}

func TestLoadConfigParsesAndExpandsTilde(t *testing.T) {
	home := t.TempDir()
	xdg := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", xdg)

	path := filepath.Join(xdg, "oak-tree", "config.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	content := []byte("root_search_dirs = [\"~/dev/general/../general\", \"~/work\"]\nroots = [\"~/src/oak-tree\"]\n")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}

	want := []string{filepath.Join(home, "dev", "general"), filepath.Join(home, "work")}
	if !reflect.DeepEqual(cfg.RootSearchDirs, want) {
		t.Fatalf("LoadConfig() dirs = %#v, want %#v", cfg.RootSearchDirs, want)
	}
	wantRoots := []string{filepath.Join(home, "src", "oak-tree")}
	if !reflect.DeepEqual(cfg.Roots, wantRoots) {
		t.Fatalf("LoadConfig() roots = %#v, want %#v", cfg.Roots, wantRoots)
	}
}

func TestLoadConfigMissingFileReturnsEmptyConfig(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.RootSearchDirs) != 0 || len(cfg.Roots) != 0 {
		t.Fatalf("LoadConfig() = %#v, want empty config", cfg)
	}
}

func TestNormalizeConfigPathExpandsTildeAndCleans(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	got, err := normalizeConfigPath("~/dev/../dev/example")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, "dev", "example")
	if got != want {
		t.Fatalf("normalizeConfigPath() = %q, want %q", got, want)
	}
}
