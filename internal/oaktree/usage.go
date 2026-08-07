package oaktree

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const UsageCacheMaxAge = time.Minute

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
	data, err := s.Exec.Output(ctx, "bunx", "ccusage", "session", "--json")
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

func ParseUsage(data []byte, now time.Time) (UsageCache, error) {
	var root map[string]any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&root); err != nil {
		return UsageCache{}, err
	}

	cache := UsageCache{RefreshedAt: now.UTC()}
	cache.TotalCostUSD = firstJSONNumber(root, "totalCostUSD", "totalCost", "costUSD")
	if totals, ok := firstJSONMap(root, "totals", "summary"); ok {
		if value := firstJSONNumber(totals, "totalCostUSD", "totalCost", "costUSD"); value > 0 {
			cache.TotalCostUSD = value
		}
	}

	for _, row := range firstJSONArray(root, "sessions", "session", "data") {
		item, ok := row.(map[string]any)
		if !ok {
			continue
		}
		session := UsageSession{
			SessionID:    firstJSONString(item, "sessionId", "sessionID", "session_id", "period", "id"),
			TotalCostUSD: firstJSONNumber(item, "totalCostUSD", "totalCost", "costUSD"),
			TotalTokens:  firstJSONInt(item, "totalTokens", "tokens"),
			ModelsUsed:   firstJSONStringArray(item, "modelsUsed", "models"),
		}
		if session.TotalTokens == 0 {
			session.TotalTokens = sumJSONInts(item, "inputTokens", "outputTokens", "cacheCreationInputTokens", "cacheReadInputTokens")
		}
		session.LastActivity = firstJSONTime(item, "lastActivity", "lastActivityAt", "updatedAt", "lastUsedAt")
		if session.SessionID == "" && session.TotalCostUSD == 0 && session.TotalTokens == 0 {
			continue
		}
		cache.Sessions = append(cache.Sessions, session)
	}

	if cache.TotalCostUSD == 0 {
		for _, session := range cache.Sessions {
			cache.TotalCostUSD += session.TotalCostUSD
		}
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
	candidate = normalizeUsageSessionID(candidate)
	if candidate == "" {
		return false
	}
	for _, id := range ids {
		id = normalizeUsageSessionID(id)
		if id == "" {
			continue
		}
		if id == candidate || strings.HasSuffix(candidate, id) || strings.HasSuffix(id, candidate) {
			return true
		}
	}
	return false
}

func normalizeUsageSessionID(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, `"'`)
	return value
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

func firstJSONMap(value map[string]any, keys ...string) (map[string]any, bool) {
	for _, key := range keys {
		if child, ok := value[key].(map[string]any); ok {
			return child, true
		}
	}
	return nil, false
}

func firstJSONArray(value map[string]any, keys ...string) []any {
	for _, key := range keys {
		if items, ok := value[key].([]any); ok {
			return items
		}
	}
	return nil
}

func firstJSONString(value map[string]any, keys ...string) string {
	for _, key := range keys {
		if text, ok := value[key].(string); ok && strings.TrimSpace(text) != "" {
			return strings.TrimSpace(text)
		}
	}
	return ""
}

func firstJSONStringArray(value map[string]any, keys ...string) []string {
	for _, key := range keys {
		raw, ok := value[key]
		if !ok {
			continue
		}
		switch typed := raw.(type) {
		case []any:
			var out []string
			for _, item := range typed {
				switch value := item.(type) {
				case string:
					if strings.TrimSpace(value) != "" {
						out = append(out, strings.TrimSpace(value))
					}
				case map[string]any:
					if model := firstJSONString(value, "model", "name", "id"); model != "" {
						out = append(out, model)
					}
				}
			}
			return dedupeStrings(out)
		case []string:
			return dedupeStrings(typed)
		}
	}
	return nil
}

func firstJSONNumber(value map[string]any, keys ...string) float64 {
	for _, key := range keys {
		raw, ok := value[key]
		if !ok {
			continue
		}
		switch typed := raw.(type) {
		case json.Number:
			parsed, err := typed.Float64()
			if err == nil {
				return parsed
			}
		case float64:
			return typed
		case int:
			return float64(typed)
		case int64:
			return float64(typed)
		case string:
			parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
			if err == nil {
				return parsed
			}
		}
	}
	return 0
}

func firstJSONInt(value map[string]any, keys ...string) int64 {
	for _, key := range keys {
		raw, ok := value[key]
		if !ok {
			continue
		}
		switch typed := raw.(type) {
		case json.Number:
			parsed, err := typed.Int64()
			if err == nil {
				return parsed
			}
			floatValue, err := typed.Float64()
			if err == nil {
				return int64(floatValue)
			}
		case float64:
			return int64(typed)
		case int:
			return int64(typed)
		case int64:
			return typed
		case string:
			parsed, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
			if err == nil {
				return parsed
			}
		}
	}
	return 0
}

func sumJSONInts(value map[string]any, keys ...string) int64 {
	var total int64
	for _, key := range keys {
		total += firstJSONInt(value, key)
	}
	return total
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
