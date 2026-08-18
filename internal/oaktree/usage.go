package oaktree

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"
)

const (
	UsageCacheMaxAge = time.Minute
	CcusageVersion   = "20.0.20"
)

var usageSessionIDPattern = regexp.MustCompile(`(?i)[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)

type UsageCache struct {
	RefreshedAt  time.Time      `json:"refreshed_at"`
	TotalCostUSD float64        `json:"total_cost_usd,omitempty"`
	Sessions     []UsageSession `json:"sessions,omitempty"`
}

type UsageSession struct {
	SessionID    string    `json:"session_id"`
	TotalCostUSD float64   `json:"total_cost_usd,omitempty"`
	TotalTokens  int64     `json:"total_tokens,omitempty"`
	LastActivity time.Time `json:"last_activity,omitempty"`
	ModelsUsed   []string  `json:"models_used,omitempty"`
}

type UsageCacheState struct {
	Cache UsageCache
	Found bool
	Stale bool
}

func (s *Service) LoadCachedUsage(maxAge time.Duration) (UsageCacheState, error) {
	cache, err := s.Store.LoadUsageCache()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return UsageCacheState{}, nil
		}
		return UsageCacheState{}, err
	}
	state := UsageCacheState{Cache: cache, Found: true}
	if cache.RefreshedAt.IsZero() || (maxAge > 0 && time.Since(cache.RefreshedAt) > maxAge) {
		state.Stale = true
	}
	return state, nil
}

func (s *Service) RefreshUsage(ctx context.Context) (UsageCache, error) {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	now := time.Now().UTC()
	data, err := s.Exec.Output(ctx, "bunx", "ccusage@"+CcusageVersion, "session", "--json")
	if err != nil {
		return UsageCache{}, err
	}
	cache, err := ParseUsage(data, now)
	if err != nil {
		return UsageCache{}, err
	}
	if err := s.Store.SaveUsageCache(cache); err != nil {
		return UsageCache{}, err
	}
	return cache, nil
}

// ParseUsage accepts only the ccusage 20.0.20 `session --json` Pi row schema.
// Do not guess aliases here: a schema change must make dashboard costs visibly unavailable.
func ParseUsage(data []byte, now time.Time) (UsageCache, error) {
	var root map[string]any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&root); err != nil {
		return UsageCache{}, err
	}
	rows, ok := root["session"].([]any)
	if !ok {
		return UsageCache{}, errors.New("ccusage session JSON missing session array")
	}

	cache := UsageCache{RefreshedAt: now.UTC()}
	seen := make(map[string]struct{})
	for index, row := range rows {
		item, ok := row.(map[string]any)
		if !ok {
			return UsageCache{}, fmt.Errorf("ccusage session row %d is not an object", index)
		}
		agent, ok := item["agent"].(string)
		if !ok {
			return UsageCache{}, fmt.Errorf("ccusage session row %d missing agent", index)
		}
		if agent != "pi" {
			continue
		}
		period, ok := item["period"].(string)
		if !ok {
			return UsageCache{}, fmt.Errorf("ccusage Pi session row %d missing period", index)
		}
		id := canonicalUsageSessionID(period)
		if id == "" {
			return UsageCache{}, fmt.Errorf("ccusage Pi session row %d has ambiguous period %q", index, period)
		}
		cost, ok := jsonNumber(item["totalCost"])
		if !ok {
			return UsageCache{}, fmt.Errorf("ccusage Pi session row %d missing numeric totalCost", index)
		}
		tokens, ok := jsonInt(item["totalTokens"])
		if !ok {
			return UsageCache{}, fmt.Errorf("ccusage Pi session row %d missing numeric totalTokens", index)
		}
		if _, duplicate := seen[id]; duplicate {
			return UsageCache{}, fmt.Errorf("ccusage returned duplicate Pi session %q", id)
		}
		seen[id] = struct{}{}
		session := UsageSession{SessionID: id, TotalCostUSD: cost, TotalTokens: tokens, ModelsUsed: firstJSONStringArray(item, "modelsUsed")}
		if metadata, ok := item["metadata"].(map[string]any); ok {
			session.LastActivity = firstJSONTime(metadata, "lastActivity")
		}
		cache.Sessions = append(cache.Sessions, session)
		cache.TotalCostUSD += cost
	}
	return cache, nil
}

func (c UsageCache) ForSessionIDs(ids []string) (UsageSession, bool) {
	ids = dedupeStrings(ids)
	if len(ids) == 0 || len(c.Sessions) == 0 {
		return UsageSession{}, false
	}

	var out UsageSession
	found := false
	models := make(map[string]struct{})
	for _, session := range c.Sessions {
		if !usageSessionIDMatches(ids, session.SessionID) {
			continue
		}
		found = true
		out.TotalCostUSD += session.TotalCostUSD
		out.TotalTokens += session.TotalTokens
		if out.SessionID == "" {
			out.SessionID = session.SessionID
		}
		if session.LastActivity.After(out.LastActivity) {
			out.LastActivity = session.LastActivity
		}
		for _, model := range session.ModelsUsed {
			model = strings.TrimSpace(model)
			if model == "" {
				continue
			}
			if _, ok := models[model]; ok {
				continue
			}
			models[model] = struct{}{}
			out.ModelsUsed = append(out.ModelsUsed, model)
		}
	}
	return out, found
}

func usageSessionIDMatches(ids []string, candidate string) bool {
	candidate = canonicalUsageSessionID(candidate)
	if candidate == "" {
		return false
	}
	for _, id := range ids {
		if canonicalUsageSessionID(id) == candidate {
			return true
		}
	}
	return false
}

// canonicalUsageSessionID accepts a raw UUID or exactly one UUID embedded in a
// known session-file path. Broad suffix matching can charge another session.
func canonicalUsageSessionID(value string) string {
	matches := usageSessionIDPattern.FindAllString(value, -1)
	if len(matches) != 1 {
		return ""
	}
	return strings.ToLower(matches[0])
}

func formatUSD(value float64) string {
	if value < 0 {
		value = 0
	}
	if value >= 1000 {
		return fmt.Sprintf("$%.0f", value)
	}
	if value >= 100 {
		return fmt.Sprintf("$%.1f", value)
	}
	return fmt.Sprintf("$%.2f", value)
}

func jsonNumber(value any) (float64, bool) {
	switch typed := value.(type) {
	case json.Number:
		parsed, err := typed.Float64()
		return parsed, err == nil
	case float64:
		return typed, true
	}
	return 0, false
}

func jsonInt(value any) (int64, bool) {
	switch typed := value.(type) {
	case json.Number:
		parsed, err := typed.Int64()
		return parsed, err == nil
	case float64:
		return int64(typed), typed == float64(int64(typed))
	}
	return 0, false
}

func firstJSONStringArray(value map[string]any, keys ...string) []string {
	for _, key := range keys {
		items, ok := value[key].([]any)
		if !ok {
			continue
		}
		out := make([]string, 0, len(items))
		for _, item := range items {
			text, ok := item.(string)
			if ok && strings.TrimSpace(text) != "" {
				out = append(out, strings.TrimSpace(text))
			}
		}
		return dedupeStrings(out)
	}
	return nil
}

func firstJSONTime(value map[string]any, keys ...string) time.Time {
	for _, key := range keys {
		raw, ok := value[key].(string)
		if !ok || strings.TrimSpace(raw) == "" {
			continue
		}
		if parsed, err := time.Parse(time.RFC3339Nano, raw); err == nil {
			return parsed
		}
		if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
			return parsed
		}
	}
	return time.Time{}
}
