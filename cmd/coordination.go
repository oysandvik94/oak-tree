package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/oysandvik94/oak-tree/internal/oaktree"
)

func newCoordinationCommand() *cobra.Command {
	var payload, actor, pane string
	command := &cobra.Command{
		Use:   "coordination",
		Short: "List or manage temporary coordination groups",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			svc, err := newService()
			if err != nil {
				return err
			}
			request := oaktree.CoordinationRequest{Action: "list"}
			if payload != "" {
				if err := json.Unmarshal([]byte(payload), &request); err != nil {
					return fmt.Errorf("read coordination request: %w", err)
				}
			}
			if actor == "" && pane != "" {
				actor, err = svc.ResolveOakSessionID(cmd.Context(), pane)
				if err != nil {
					return err
				}
			}
			if actor != "" {
				sessions, err := svc.Store.ListSessions()
				if err != nil {
					return err
				}
				found := false
				for _, session := range sessions {
					found = found || session.ID == actor
				}
				if !found {
					return fmt.Errorf("calling session is no longer available")
				}
			}
			request.ActorID = actor
			if request.Action != "list" {
				if request.Action == "report" && request.SessionID == "" {
					request.SessionID = actor
				}
				group, err := svc.ApplyCoordination(request)
				if err != nil {
					return err
				}
				return json.NewEncoder(cmd.OutOrStdout()).Encode(group)
			}
			groups, err := svc.Store.LoadCoordinationGroups()
			if err != nil {
				return err
			}
			active := make([]oaktree.CoordinationGroup, 0, len(groups))
			for _, group := range groups {
				if group.ClosedAt == nil && (request.GroupID == "" || group.ID == request.GroupID) {
					active = append(active, group)
				}
			}
			sessions, err := svc.Store.ListSessions()
			if err != nil {
				return err
			}
			type sessionIdentity struct {
				ID     string `json:"id"`
				Root   string `json:"root"`
				Branch string `json:"branch"`
				Tmux   string `json:"tmux_session"`
			}
			identities := make([]sessionIdentity, 0, len(sessions))
			for _, session := range sessions {
				if request.GroupID != "" && (len(active) == 0 || !active[0].Contains(session.ID)) {
					continue
				}
				identities = append(identities, sessionIdentity{session.ID, session.Root, session.Branch, session.TmuxSessionName})
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(struct {
				ActorID  string                      `json:"actor_id,omitempty"`
				Groups   []oaktree.CoordinationGroup `json:"groups"`
				Sessions []sessionIdentity           `json:"sessions"`
			}{actor, active, identities})
		},
	}
	command.Flags().StringVar(&payload, "request", "", "Coordination request as JSON (defaults to list)")
	command.Flags().StringVar(&actor, "oak-session", "", "Calling managed session id; omitted for user actions")
	command.Flags().StringVar(&pane, "tmux-pane", "", "Resolve the calling managed session from this pane")
	return command
}
