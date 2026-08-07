package oaktree

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

type Config struct {
	RootSearchDirs []string `toml:"root_search_dirs"`
	Roots          []string `toml:"roots"`
}

func LoadConfig() (Config, error) {
	path, err := configFilePath()
	if err != nil {
		return Config{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Config{}, nil
		}
		return Config{}, err
	}
	var cfg Config
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", path, err)
	}
	cfg.RootSearchDirs, err = normalizeConfigPaths(cfg.RootSearchDirs)
	if err != nil {
		return Config{}, err
	}
	cfg.Roots, err = normalizeConfigPaths(cfg.Roots)
	if err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func normalizeConfigPaths(paths []string) ([]string, error) {
	normalized := make([]string, 0, len(paths))
	for _, path := range paths {
		path, err := normalizeConfigPath(path)
		if err != nil {
			return nil, err
		}
		if path != "" {
			normalized = append(normalized, path)
		}
	}
	return normalized, nil
}

func configFilePath() (string, error) {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".config")
	}
	path := filepath.Join(base, "oak-tree", "config.toml")
	return filepath.Abs(filepath.Clean(path))
}

func expandTilde(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", nil
	}
	if path == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return home, nil
	}
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, path[2:]), nil
	}
	return path, nil
}

func normalizeConfigPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", nil
	}
	expanded, err := expandTilde(path)
	if err != nil {
		return "", err
	}
	cleaned := filepath.Clean(expanded)
	if filepath.IsAbs(cleaned) {
		return cleaned, nil
	}
	return filepath.Abs(cleaned)
}
