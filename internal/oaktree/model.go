package oaktree

import "time"

type AgentStatus string

const (
	AgentStatusUnknown   AgentStatus = ""
	AgentStatusWorking   AgentStatus = "working"
	AgentStatusAttention AgentStatus = "attention"
	AgentStatusQuestion  AgentStatus = "question"
	AgentStatusIdle      AgentStatus = "idle"
)

type SessionTag string

const (
	SessionTagNone          SessionTag = ""
	SessionTagWaitingReview SessionTag = "waiting_review"
	SessionTagTesting       SessionTag = "testing"
)

type TodoTask struct {
	Subject string `json:"subject"`
	Status  string `json:"status"`
}

type TodoSummary struct {
	Total      int        `json:"total"`
	Pending    int        `json:"pending"`
	InProgress int        `json:"in_progress"`
	Completed  int        `json:"completed"`
	Tasks      []TodoTask `json:"tasks,omitempty"`
}

type Session struct {
	ID                   string       `json:"id"`
	Root                 string       `json:"root"`
	Workdir              string       `json:"workdir"`
	RepoKey              string       `json:"repo_key"`
	Branch               string       `json:"branch,omitempty"`
	OwnedWorktree        bool         `json:"owned_worktree"`
	TmuxSessionName      string       `json:"tmux_session_name"`
	TmuxSessionID        string       `json:"tmux_session_id,omitempty"`
	LeftPaneID           string       `json:"left_pane_id,omitempty"`
	RightPaneID          string       `json:"right_pane_id,omitempty"`
	AgentSessionIDs      []string     `json:"agent_session_ids,omitempty"`
	AgentSessionFile     string       `json:"agent_session_file,omitempty"`
	AgentStatus          AgentStatus  `json:"agent_status,omitempty"`
	AgentStatusUpdatedAt *time.Time   `json:"agent_status_updated_at,omitempty"`
	Tag                  SessionTag   `json:"tag,omitempty"`
	TagUpdatedAt         *time.Time   `json:"tag_updated_at,omitempty"`
	Note                 string       `json:"note,omitempty"`
	Todo                 *TodoSummary `json:"todo,omitempty"`
	GitStatus            *GitStatus   `json:"-"`
	PR                   *PRInfo      `json:"pr,omitempty"`
	CreatedAt            time.Time    `json:"created_at"`
	UpdatedAt            time.Time    `json:"updated_at"`
	LastHookAt           *time.Time   `json:"last_hook_at,omitempty"`
	LastHookCwd          string       `json:"last_hook_cwd,omitempty"`
}

type GitStatus struct {
	RefreshedAt time.Time `json:"refreshed_at"`
	Branch      string    `json:"-"`
	Clean       bool      `json:"clean"`
	Changed     int       `json:"changed,omitempty"`
	Untracked   int       `json:"untracked,omitempty"`
	Ahead       int       `json:"ahead,omitempty"`
	Behind      int       `json:"behind,omitempty"`
	Error       string    `json:"error,omitempty"`
}

type PRInfo struct {
	RefreshedAt               time.Time `json:"refreshed_at"`
	Found                     bool      `json:"found"`
	Number                    int       `json:"number,omitempty"`
	URL                       string    `json:"url,omitempty"`
	Title                     string    `json:"title,omitempty"`
	State                     string    `json:"state,omitempty"`
	IsDraft                   bool      `json:"is_draft,omitempty"`
	ReviewDecision            string    `json:"review_decision,omitempty"`
	ChecksState               string    `json:"checks_state,omitempty"`
	UnresolvedComments        *int      `json:"unresolved_comments,omitempty"`
	UnresolvedCommentsChecked bool      `json:"unresolved_comments_checked,omitempty"`
	UpdatedAt                 time.Time `json:"updated_at,omitempty"`
}
