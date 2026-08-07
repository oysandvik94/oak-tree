package oaktree

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestParseGitHubRemote(t *testing.T) {
	tests := []struct {
		name   string
		remote string
		want   string
	}{
		{
			name:   "ssh scp",
			remote: "git@github.com:example/example-repo.git",
			want:   "example/example-repo",
		},
		{
			name:   "https",
			remote: "https://github.com/oysandvik94/oak-tree.git",
			want:   "oysandvik94/oak-tree",
		},
		{
			name:   "enterprise ssh url",
			remote: "ssh://git@git.example.com/platform/service.git",
			want:   "git.example.com/platform/service",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, err := ParseGitHubRemote(tt.remote)
			if err != nil {
				t.Fatal(err)
			}
			if got := repo.CLIName(); got != tt.want {
				t.Fatalf("CLIName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseGHPRListSummarizesStatus(t *testing.T) {
	now := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)
	data := []byte(`[
		{
			"number": 42,
			"url": "https://github.com/oysandvik94/oak-tree/pull/42",
			"title": "Add PR cache",
			"state": "OPEN",
			"isDraft": false,
			"reviewDecision": "APPROVED",
			"updatedAt": "2026-06-25T11:00:00Z",
			"statusCheckRollup": [
				{"status": "COMPLETED", "conclusion": "SUCCESS"},
				{"state": "SUCCESS"}
			]
		}
	]`)
	prs, err := parseGHPRList(data, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(prs) != 1 {
		t.Fatalf("parseGHPRList() len = %d, want 1", len(prs))
	}
	got := prs[0]
	if !got.Found || got.Number != 42 || got.ChecksState != "pass" || got.ReviewDecision != "approved" {
		t.Fatalf("parseGHPRList() = %#v, want found pass approved", got)
	}
}

func TestRefreshPullRequestInfoKeepsPRWhenCommentsUnavailable(t *testing.T) {
	runner := &stubRunner{outputFunc: func(name string, args []string) ([]byte, error) {
		if name == "git" {
			return []byte("git@github.com:oysandvik94/oak-tree.git\n"), nil
		}
		if argsContain(args, "api") {
			return nil, errors.New("review threads unavailable")
		}
		return []byte(`[{"number":42,"url":"https://github.com/oysandvik94/oak-tree/pull/42","state":"MERGED","statusCheckRollup":[]}]`), nil
	}}

	info, err := RefreshPullRequestInfo(context.Background(), runner, "/repo", "feature/pr", testTime())
	if err != nil {
		t.Fatal(err)
	}
	if !info.Found || info.Number != 42 || info.UnresolvedComments != nil || !info.UnresolvedCommentsChecked {
		t.Fatalf("RefreshPullRequestInfo() = %#v, want cached PR with unavailable comments marked checked", info)
	}
}

func TestFetchUnresolvedCommentsCountsPaginatedThreads(t *testing.T) {
	runner := &stubRunner{outputFunc: func(name string, args []string) ([]byte, error) {
		if name != "gh" || !argsContain(args, "api") || !argsContain(args, "--paginate") || !argsContain(args, "git.example.com") {
			t.Fatalf("command = %s %#v, want paginated GitHub GraphQL request", name, args)
		}
		return []byte(`[
			{"data":{"repository":{"pullRequest":{"reviewThreads":{"nodes":[{"isResolved":false},{"isResolved":true}]}}}}},
			{"data":{"repository":{"pullRequest":{"reviewThreads":{"nodes":[{"isResolved":false}]}}}}}
		]`), nil
	}}

	got, err := fetchUnresolvedComments(context.Background(), runner, GitHubRepo{Host: "git.example.com", Owner: "platform", Name: "service"}, 42)
	if err != nil {
		t.Fatal(err)
	}
	if got != 2 {
		t.Fatalf("fetchUnresolvedComments() = %d, want 2", got)
	}
}

func TestSummarizeStatusCheckRollupFailureWins(t *testing.T) {
	raw := []json.RawMessage{
		json.RawMessage(`{"status":"COMPLETED","conclusion":"SUCCESS"}`),
		json.RawMessage(`{"status":"COMPLETED","conclusion":"FAILURE"}`),
	}
	if got := summarizeStatusCheckRollup(raw); got != "fail" {
		t.Fatalf("summarizeStatusCheckRollup() = %q, want fail", got)
	}
}

func TestSummarizeStatusCheckRollupPending(t *testing.T) {
	raw := []json.RawMessage{
		json.RawMessage(`{"status":"COMPLETED","conclusion":"SUCCESS"}`),
		json.RawMessage(`{"status":"IN_PROGRESS"}`),
	}
	if got := summarizeStatusCheckRollup(raw); got != "pending" {
		t.Fatalf("summarizeStatusCheckRollup() = %q, want pending", got)
	}
}
