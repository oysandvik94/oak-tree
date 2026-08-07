package oaktree

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type Paths struct {
	StateDir     string
	SessionsDir  string
	WorktreesDir string
	HooksDir     string
	PiDir        string
}

func DefaultPaths(explicit string) (Paths, error) {
	stateDir := explicit
	if stateDir == "" {
		stateDir = os.Getenv("XDG_STATE_HOME")
		if stateDir == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return Paths{}, err
			}
			stateDir = filepath.Join(home, ".local", "state")
		}
		stateDir = filepath.Join(stateDir, "oak-tree")
	}
	stateDir, err := filepath.Abs(stateDir)
	if err != nil {
		return Paths{}, err
	}
	return Paths{
		StateDir:     stateDir,
		SessionsDir:  filepath.Join(stateDir, "sessions"),
		WorktreesDir: filepath.Join(stateDir, "worktrees"),
		HooksDir:     filepath.Join(stateDir, "hooks"),
		PiDir:        filepath.Join(stateDir, "pi"),
	}, nil
}

func SafeComponent(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "default"
	}
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if ok {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "default"
	}
	return out
}

func RepoKeyFromRoot(root string) (string, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	abs = filepath.Clean(abs)
	base := filepath.Base(abs)
	sum := sha256.Sum256([]byte(abs))
	return fmt.Sprintf("%s-%s", SafeComponent(base), hex.EncodeToString(sum[:4])), nil
}

func SessionFilePath(stateDir, id string) string {
	return filepath.Join(stateDir, "sessions", id+".json")
}

func UsageCacheFilePath(stateDir string) string {
	return filepath.Join(stateDir, "cache", "usage.json")
}

func DashboardPreferencesFilePath(stateDir string) string {
	return filepath.Join(stateDir, "dashboard.json")
}

func WorktreePath(stateDir, repoKey, branch string) string {
	return filepath.Join(stateDir, "worktrees", repoKey, SafeComponent(branch))
}

func CleanComparablePath(path string) string {
	if path == "" {
		return ""
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}
	if runtime.GOOS == "windows" {
		return strings.ToLower(filepath.Clean(abs))
	}
	return filepath.Clean(abs)
}

func SamePath(a, b string) bool {
	return CleanComparablePath(a) == CleanComparablePath(b)
}
