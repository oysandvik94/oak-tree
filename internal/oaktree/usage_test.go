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
		"session": [{
			"agent": "pi",
			"period": "/home/user/sessions/019fccbc-db8a-7ebd-af1d-19f562cf7927.jsonl",
			"totalCost": 1.23,
			"totalTokens": 140,
			"modelsUsed": ["[pi] gpt-5.6-sol"],
			"metadata": {"lastActivity": "2026-06-25T12:30:00Z"}
		}]
	}`), now)
	if err != nil {
		t.Fatal(err)
	}
	if !cache.RefreshedAt.Equal(now.UTC()) || cache.TotalCostUSD != 1.23 || len(cache.Sessions) != 1 {
		t.Fatalf("cache = %#v, want one parsed Pi session", cache)
	}
	session := cache.Sessions[0]
	if session.SessionID != "019fccbc-db8a-7ebd-af1d-19f562cf7927" || session.TotalTokens != 140 || len(session.ModelsUsed) != 1 {
		t.Fatalf("session = %#v, want canonical parsed session", session)
	}
}

func TestParseUsageRejectsSchemaDrift(t *testing.T) {
	_, err := ParseUsage([]byte(`{"sessions":[{"sessionId":"pi-1"}]}`), testTime())
	if err == nil {
		t.Fatal("ParseUsage() error = nil, want schema error")
	}
}

func TestUsageCacheForSessionIDsUsesExactCanonicalIDs(t *testing.T) {
	first := "019fccbc-db8a-7ebd-af1d-19f562cf7927"
	second := "019fccc2-e93f-7b01-8247-7393ea4f0c4f"
	cache := UsageCache{Sessions: []UsageSession{
		{SessionID: first, TotalCostUSD: 1.20, TotalTokens: 100, ModelsUsed: []string{"one"}},
		{SessionID: second, TotalCostUSD: 2.30, TotalTokens: 200, ModelsUsed: []string{"two"}},
	}}

	usage, ok := cache.ForSessionIDs([]string{first, first})
	if !ok || usage.TotalCostUSD != 1.20 || usage.TotalTokens != 100 {
		t.Fatalf("usage = %#v, want only one explicit session", usage)
	}
	if _, ok := cache.ForSessionIDs([]string{"prefix-" + first[:8]}); ok {
		t.Fatal("ForSessionIDs() matched a prefix collision")
	}
	usage, ok = cache.ForSessionIDs([]string{"/tmp/" + first + ".jsonl", second})
	if !ok || usage.TotalCostUSD != 3.50 || usage.TotalTokens != 300 {
		t.Fatalf("usage = %#v, want two explicit canonical IDs", usage)
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
			want := []string{"ccusage@" + CcusageVersion, "session", "--json"}
			if !stringSlicesEqual(args, want) {
				t.Fatalf("args = %#v, want %#v", args, want)
			}
			return []byte(`{"session":[{"agent":"pi","period":"019fccbc-db8a-7ebd-af1d-19f562cf7927","totalCost":4.56,"totalTokens":10}]}`), nil
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
	if len(loaded.Sessions) != 1 || loaded.Sessions[0].SessionID != "019fccbc-db8a-7ebd-af1d-19f562cf7927" {
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
