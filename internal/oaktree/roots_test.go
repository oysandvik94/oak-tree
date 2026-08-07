package oaktree

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestDiscoverRootCandidatesImmediateChildrenOnly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	first := filepath.Join(home, "dev", "general")
	second := filepath.Join(home, "work")
	if err := os.MkdirAll(filepath.Join(first, "alpha"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(first, "beta", "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(first, "file.txt"), []byte("ignored"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(second, "gamma"), 0o755); err != nil {
		t.Fatal(err)
	}

	candidates := discoverRootCandidates([]string{
		"~/dev/general",
		"~/dev/missing",
		"~/work",
	}, nil)

	got := candidatePaths(candidates)
	want := []string{
		filepath.Join(first, "alpha"),
		filepath.Join(first, "beta"),
		filepath.Join(second, "gamma"),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("discoverRootCandidates() = %#v, want %#v", got, want)
	}
}

func TestDiscoverRootCandidatesIncludesConfiguredRoots(t *testing.T) {
	base := t.TempDir()
	searchDir := filepath.Join(base, "repos")
	discovered := filepath.Join(searchDir, "discovered")
	explicit := filepath.Join(base, "standalone")
	if err := os.MkdirAll(discovered, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(explicit, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(base, "file.txt")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	candidates := discoverRootCandidates([]string{searchDir}, []string{
		explicit,
		discovered,
		filepath.Join(base, "missing"),
		file,
	})

	want := []string{discovered, explicit}
	if got := candidatePaths(candidates); !reflect.DeepEqual(got, want) {
		t.Fatalf("discoverRootCandidates() = %#v, want %#v", got, want)
	}
}

func TestFilterRootCandidatesEmptyQueryReturnsAll(t *testing.T) {
	candidates := []rootCandidate{
		{Path: "/tmp/a", SearchDir: "/tmp"},
		{Path: "/tmp/b", SearchDir: "/tmp"},
	}

	filtered := filterRootCandidates("", candidates)
	got := candidatePaths(filtered)
	want := candidatePaths(candidates)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("filterRootCandidates() = %#v, want %#v", got, want)
	}
}

func TestRootCandidateDisplayUsesNameAndCompactPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	candidate := rootCandidate{
		Path:      filepath.Join(home, "dev", "general", "oak-tree"),
		SearchDir: filepath.Join(home, "dev", "general"),
	}
	if got := candidate.Title(); got != "oak-tree" {
		t.Fatalf("Title() = %q, want oak-tree", got)
	}
	description := candidate.Description()
	if !strings.HasPrefix(description, "~") {
		t.Fatalf("Description() = %q, want compact home-relative path", description)
	}
	if strings.Contains(description, home) {
		t.Fatalf("Description() = %q, should not expose full home path", description)
	}
}

func candidatePaths(candidates []rootCandidate) []string {
	paths := make([]string, len(candidates))
	for i := range candidates {
		paths[i] = candidates[i].Path
	}
	return paths
}
