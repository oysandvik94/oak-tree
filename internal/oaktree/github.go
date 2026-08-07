package oaktree

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path"
	"strings"
	"time"
)

type GitHubRepo struct {
	Host  string
	Owner string
	Name  string
}

func (r GitHubRepo) CLIName() string {
	if r.Host == "" || r.Host == "github.com" {
		return r.Owner + "/" + r.Name
	}
	return r.Host + "/" + r.Owner + "/" + r.Name
}

func RefreshPullRequestInfo(ctx context.Context, runner Runner, workdir, branch string, now time.Time) (*PRInfo, error) {
	info := &PRInfo{RefreshedAt: now.UTC(), Found: false}
	branch = strings.TrimSpace(branch)
	if branch == "" {
		return info, nil
	}
	remote, err := runner.Output(ctx, "git", "-C", workdir, "remote", "get-url", "origin")
	if err != nil {
		return nil, err
	}
	repo, err := ParseGitHubRemote(strings.TrimSpace(string(remote)))
	if err != nil {
		return nil, err
	}
	data, err := runner.Output(ctx, "gh", "pr", "list",
		"--repo", repo.CLIName(),
		"--head", branch,
		"--state", "all",
		"--limit", "1",
		"--json", "number,url,title,state,isDraft,reviewDecision,statusCheckRollup,updatedAt",
	)
	if err != nil {
		return nil, err
	}
	prs, err := parseGHPRList(data, now)
	if err != nil {
		return nil, err
	}
	if len(prs) == 0 {
		return info, nil
	}
	info = prs[0]
	info.UnresolvedCommentsChecked = true
	if unresolved, err := fetchUnresolvedComments(ctx, runner, repo, info.Number); err == nil {
		info.UnresolvedComments = &unresolved
	}
	return info, nil
}

const unresolvedCommentsQuery = `query($owner:String!,$name:String!,$number:Int!,$endCursor:String){
  repository(owner:$owner,name:$name){
    pullRequest(number:$number){
      reviewThreads(first:100,after:$endCursor){
        nodes{isResolved}
        pageInfo{hasNextPage endCursor}
      }
    }
  }
}`

type ghReviewThreadsPage struct {
	Data struct {
		Repository struct {
			PullRequest *struct {
				ReviewThreads struct {
					Nodes []struct {
						IsResolved bool `json:"isResolved"`
					} `json:"nodes"`
				} `json:"reviewThreads"`
			} `json:"pullRequest"`
		} `json:"repository"`
	} `json:"data"`
}

func fetchUnresolvedComments(ctx context.Context, runner Runner, repo GitHubRepo, number int) (int, error) {
	data, err := runner.Output(ctx, "gh", "api", "graphql",
		"--hostname", repo.Host,
		"--paginate", "--slurp",
		"-F", "owner="+repo.Owner,
		"-F", "name="+repo.Name,
		"-F", fmt.Sprintf("number=%d", number),
		"-f", "query="+unresolvedCommentsQuery,
	)
	if err != nil {
		return 0, err
	}
	var pages []ghReviewThreadsPage
	if err := json.Unmarshal(data, &pages); err != nil {
		return 0, err
	}
	if len(pages) == 0 {
		return 0, errors.New("GitHub returned no review thread data")
	}
	unresolved := 0
	for _, page := range pages {
		if page.Data.Repository.PullRequest == nil {
			return 0, errors.New("GitHub returned no pull request review threads")
		}
		for _, thread := range page.Data.Repository.PullRequest.ReviewThreads.Nodes {
			if !thread.IsResolved {
				unresolved++
			}
		}
	}
	return unresolved, nil
}

func ParseGitHubRemote(remote string) (GitHubRepo, error) {
	remote = strings.TrimSpace(remote)
	if remote == "" {
		return GitHubRepo{}, errors.New("empty git remote")
	}
	if strings.HasPrefix(remote, "git@") {
		parts := strings.SplitN(strings.TrimPrefix(remote, "git@"), ":", 2)
		if len(parts) == 2 {
			return githubRepoFromHostPath(parts[0], parts[1])
		}
	}
	if parsed, err := url.Parse(remote); err == nil && parsed.Host != "" {
		return githubRepoFromHostPath(parsed.Host, parsed.Path)
	}
	return GitHubRepo{}, fmt.Errorf("unsupported GitHub remote %q", remote)
}

func githubRepoFromHostPath(host, repoPath string) (GitHubRepo, error) {
	host = strings.TrimSpace(strings.TrimPrefix(host, "ssh://"))
	repoPath = strings.Trim(path.Clean(strings.TrimPrefix(repoPath, "/")), "/")
	repoPath = strings.TrimSuffix(repoPath, ".git")
	parts := strings.Split(repoPath, "/")
	if host == "" || len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return GitHubRepo{}, fmt.Errorf("unsupported GitHub remote path %q", repoPath)
	}
	return GitHubRepo{Host: host, Owner: parts[0], Name: parts[1]}, nil
}

type ghPR struct {
	Number            int               `json:"number"`
	URL               string            `json:"url"`
	Title             string            `json:"title"`
	State             string            `json:"state"`
	IsDraft           bool              `json:"isDraft"`
	ReviewDecision    string            `json:"reviewDecision"`
	StatusCheckRollup []json.RawMessage `json:"statusCheckRollup"`
	UpdatedAt         string            `json:"updatedAt"`
}

func parseGHPRList(data []byte, now time.Time) ([]*PRInfo, error) {
	var prs []ghPR
	if err := json.Unmarshal(data, &prs); err != nil {
		return nil, err
	}
	out := make([]*PRInfo, 0, len(prs))
	for _, pr := range prs {
		info := &PRInfo{
			RefreshedAt:    now.UTC(),
			Found:          true,
			Number:         pr.Number,
			URL:            strings.TrimSpace(pr.URL),
			Title:          strings.TrimSpace(pr.Title),
			State:          strings.TrimSpace(pr.State),
			IsDraft:        pr.IsDraft,
			ReviewDecision: normalizeReviewDecision(pr.ReviewDecision),
			ChecksState:    summarizeStatusCheckRollup(pr.StatusCheckRollup),
		}
		if pr.UpdatedAt != "" {
			if updated, err := time.Parse(time.RFC3339, pr.UpdatedAt); err == nil {
				info.UpdatedAt = updated
			}
		}
		out = append(out, info)
	}
	return out, nil
}

func normalizeReviewDecision(value string) string {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "APPROVED":
		return "approved"
	case "CHANGES_REQUESTED":
		return "changes_requested"
	case "REVIEW_REQUIRED":
		return "review_required"
	default:
		return "unknown"
	}
}

func summarizeStatusCheckRollup(items []json.RawMessage) string {
	if len(items) == 0 {
		return "unknown"
	}
	seenCompleted := false
	for _, item := range items {
		state := summarizeStatusCheck(item)
		switch state {
		case "fail":
			return "fail"
		case "pending":
			return "pending"
		case "pass":
			seenCompleted = true
		default:
			return "unknown"
		}
	}
	if seenCompleted {
		return "pass"
	}
	return "unknown"
}

func summarizeStatusCheck(item json.RawMessage) string {
	var fields map[string]any
	if err := json.Unmarshal(item, &fields); err != nil {
		return "unknown"
	}
	for _, key := range []string{"conclusion", "state"} {
		if value, ok := fields[key].(string); ok {
			switch strings.ToUpper(strings.TrimSpace(value)) {
			case "SUCCESS", "PASSED", "PASS", "NEUTRAL", "SKIPPED":
				return "pass"
			case "FAILURE", "FAILED", "FAIL", "ERROR", "TIMED_OUT", "ACTION_REQUIRED", "CANCELLED", "CANCELED", "STARTUP_FAILURE":
				return "fail"
			case "PENDING", "EXPECTED", "QUEUED", "IN_PROGRESS", "REQUESTED", "WAITING":
				return "pending"
			}
		}
	}
	if value, ok := fields["status"].(string); ok {
		switch strings.ToUpper(strings.TrimSpace(value)) {
		case "COMPLETED":
			return "pass"
		case "PENDING", "QUEUED", "IN_PROGRESS", "REQUESTED", "WAITING":
			return "pending"
		}
	}
	return "unknown"
}
