package oaktree

import (
	"os"
	"path/filepath"
	"strings"

	"charm.land/bubbles/v2/list"
)

type rootCandidate struct {
	Path      string
	SearchDir string
}

func (c rootCandidate) Title() string {
	if strings.TrimSpace(c.Path) == "" {
		return "root"
	}
	base := filepath.Base(filepath.Clean(c.Path))
	if base == "." || base == string(filepath.Separator) || base == "" {
		return compactPath(c.Path, 32)
	}
	return base
}

func (c rootCandidate) Description() string {
	return compactPath(c.Path, 72)
}

func (c rootCandidate) FilterValue() string {
	return strings.ToLower(c.Path + " " + c.SearchDir)
}

func discoverRootCandidates(searchDirs, roots []string) []rootCandidate {
	seen := make(map[string]struct{})
	candidates := make([]rootCandidate, 0)
	add := func(path, searchDir string) {
		key := CleanComparablePath(path)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		candidates = append(candidates, rootCandidate{Path: path, SearchDir: searchDir})
	}
	for _, rawDir := range searchDirs {
		searchDir, err := normalizeConfigPath(rawDir)
		if err != nil || searchDir == "" {
			continue
		}
		entries, err := os.ReadDir(searchDir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() {
				add(filepath.Join(searchDir, entry.Name()), searchDir)
			}
		}
	}
	for _, rawRoot := range roots {
		root, err := normalizeConfigPath(rawRoot)
		if err != nil || root == "" {
			continue
		}
		info, err := os.Stat(root)
		if err == nil && info.IsDir() {
			add(root, "")
		}
	}
	return candidates
}

func filterRootCandidates(query string, candidates []rootCandidate) []rootCandidate {
	query = strings.TrimSpace(query)
	if query == "" {
		return append([]rootCandidate(nil), candidates...)
	}
	targets := make([]string, len(candidates))
	for i, candidate := range candidates {
		targets[i] = candidate.FilterValue()
	}
	ranks := list.DefaultFilter(query, targets)
	filtered := make([]rootCandidate, 0, len(ranks))
	for _, rank := range ranks {
		if rank.Index < 0 || rank.Index >= len(candidates) {
			continue
		}
		filtered = append(filtered, candidates[rank.Index])
	}
	return filtered
}
