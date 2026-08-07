package oaktree

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

func TestParseUsageSessionJSON(t *testing.T) {
	now := testTime()
	cache, err := ParseUsage([]byte(`{
		"totals": {"totalCost": 9.87},
		"sessions": [
			{
				"sessionId": "pi-1",
				"totalCost": 1.23,
				"inputTokens": 100,
				"outputTokens": 25,
				"cacheCreationInputTokens": 5,
				"cacheReadInputTokens": 10,
				"modelsUsed": ["[pi] gpt-5.6-sol"],
				"lastActivity": "2026-06-25T12:30:00Z"
			}
		]
	}`), now)
	if err != nil {
		t.Fatal(err)
	}
	if !cache.RefreshedAt.Equal(now.UTC()) {
		t.Fatalf("RefreshedAt = %s, want %s", cache.RefreshedAt, now.UTC())
	}
	if cache.TotalCostUSD != 9.87 {
		t.Fatalf("TotalCostUSD = %v, want 9.87", cache.TotalCostUSD)
	}
	if len(cache.Sessions) != 1 {
		t.Fatalf("Sessions len = %d, want 1", len(cache.Sessions))
	}
	session := cache.Sessions[0]
	if session.SessionID != "pi-1" || session.TotalCostUSD != 1.23 || session.TotalTokens != 140 {
		t.Fatalf("session = %#v, want parsed cost and token total", session)
	}
	if len(session.ModelsUsed) != 1 || session.ModelsUsed[0] != "[pi] gpt-5.6-sol" {
		t.Fatalf("ModelsUsed = %#v, want [pi] gpt-5.6-sol", session.ModelsUsed)
	}
}

func TestParseUsageUnifiedPiSessionJSON(t *testing.T) {
	cache, err := ParseUsage([]byte(`{
		"totals": {"totalCost": 4.56},
		"session": [{
			"agent": "pi",
			"period": "pi-1",
			"totalCost": 1.23,
			"totalTokens": 140,
			"modelsUsed": ["[pi] gpt-5.6-sol"]
		}]
	}`), testTime())
	if err != nil {
		t.Fatal(err)
	}
	if len(cache.Sessions) != 1 || cache.Sessions[0].SessionID != "pi-1" || cache.Sessions[0].TotalCostUSD != 1.23 {
		t.Fatalf("session = %#v, want parsed Pi usage", cache.Sessions)
	}
}

func TestUsageCacheForSessionIDsMergesMatches(t *testing.T) {
	cache := UsageCache{
		Sessions: []UsageSession{
			{SessionID: "pi-1", TotalCostUSD: 1.20, TotalTokens: 100, ModelsUsed: []string{"one"}},
			{SessionID: "path/pi-1", TotalCostUSD: 2.30, TotalTokens: 200, ModelsUsed: []string{"one", "two"}},
			{SessionID: "pi-2", TotalCostUSD: 9},
		},
	}

	usage, ok := cache.ForSessionIDs([]string{"pi-1"})
	if !ok {
		t.Fatal("ForSessionIDs() ok = false, want true")
	}
	if usage.TotalCostUSD != 3.50 || usage.TotalTokens != 300 {
		t.Fatalf("usage = %#v, want merged matching rows", usage)
	}
	if len(usage.ModelsUsed) != 2 {
		t.Fatalf("ModelsUsed = %#v, want deduped models", usage.ModelsUsed)
	}
}

func TestRefreshUsageRunsBunxAndCachesResult(t *testing.T) {
	stateDir := t.TempDir()
	paths := Paths{StateDir: stateDir}
	store := NewStore(stateDir)
	runner := &stubRunner{
		outputFunc: func(name string, args []string) ([]byte, error) {
			if name != "bunx" {
				t.Fatalf("command = %s, want bunx", name)
			}
			want := []string{"ccusage", "session", "--json"}
			if !stringSlicesEqual(args, want) {
				t.Fatalf("args = %#v, want %#v", args, want)
			}
			return []byte(`{"session":[{"agent":"pi","period":"pi-1","totalCost":4.56}]}`), nil
		},
	}
	svc := NewService(paths, store, runner)

	cache, err := svc.RefreshUsage(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(cache.Sessions) != 1 || cache.Sessions[0].TotalCostUSD != 4.56 {
		t.Fatalf("RefreshUsage() = %#v, want cached session cost", cache)
	}
	loaded, err := store.LoadUsageCache()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Sessions) != 1 || loaded.Sessions[0].SessionID != "pi-1" {
		t.Fatalf("loaded cache = %#v, want persisted usage", loaded)
	}
	if _, err := os.Stat(UsageCacheFilePath(stateDir)); err != nil {
		t.Fatalf("usage cache file missing: %v", err)
	}
}

func TestLoadCachedUsageMarksStaleWithoutRunningCommand(t *testing.T) {
	stateDir := t.TempDir()
	store := NewStore(stateDir)
	cache := UsageCache{
		RefreshedAt: time.Now().Add(-2 * UsageCacheMaxAge),
		Sessions:    []UsageSession{{SessionID: "pi-1", TotalCostUSD: 1}},
	}
	if err := store.SaveUsageCache(cache); err != nil {
		t.Fatal(err)
	}
	svc := NewService(Paths{StateDir: stateDir}, store, &stubRunner{
		outputFunc: func(name string, args []string) ([]byte, error) {
			return nil, errors.New("unexpected command")
		},
	})

	state, err := svc.LoadCachedUsage(UsageCacheMaxAge)
	if err != nil {
		t.Fatal(err)
	}
	if !state.Found || !state.Stale {
		t.Fatalf("state = %#v, want found stale cache", state)
	}
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
