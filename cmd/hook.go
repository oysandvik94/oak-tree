package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/oysandvik94/oak-tree/internal/oaktree"
)

func newHookCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "hook",
		SilenceUsage: true,
		Short:        "Pi lifecycle hook entrypoint",
	}
	cmd.AddCommand(newAgentEventHookCommand(), newCampfireReadHookCommand())
	return cmd
}

func newCampfireReadHookCommand() *cobra.Command {
	return &cobra.Command{
		Use:          "campfire-read",
		SilenceUsage: true,
		Short:        "Print the latest Campfire updates as JSON",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := newService()
			if err != nil {
				return err
			}
			messages, err := svc.Store.LoadCampfire()
			if err != nil {
				return fmt.Errorf("load Campfire: %w", err)
			}
			const limit = 12
			if len(messages) > limit {
				messages = messages[len(messages)-limit:]
			}
			recent := make([]oaktree.CampfireMessage, len(messages))
			for i := range messages {
				recent[i] = messages[len(messages)-1-i]
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(recent)
		},
	}
}

func newAgentEventHookCommand() *cobra.Command {
	var quiet bool
	var oakSession, tmuxPane, eventName, cwd, sessionID, sessionFile, todoJSON, activityKind, activityMessage, legacyAgent string
	var todoTotal, todoPending, todoInProgress, todoCompleted int
	cmd := &cobra.Command{
		Use:          "agent-event",
		SilenceUsage: true,
		Short:        "Update oak-tree from a Pi lifecycle event",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := newService()
			if err != nil {
				if quiet {
					return nil
				}
				return err
			}
			var event oaktree.AgentEvent
			if oakSession == "" && eventName == "" {
				event, err = oaktree.ParseAgentEvent(os.Stdin)
			} else {
				event.OakSessionID, event.TmuxPaneID, event.Event, event.Cwd, event.SessionID, event.SessionFile = oakSession, tmuxPane, eventName, cwd, sessionID, sessionFile
				event.ActivityKind, event.ActivityMessage = activityKind, activityMessage
				if eventName == "todo" {
					event.Todo = &oaktree.TodoSummary{Total: todoTotal, Pending: todoPending, InProgress: todoInProgress, Completed: todoCompleted}
					if todoJSON != "" {
						err = json.Unmarshal([]byte(todoJSON), &event.Todo.Tasks)
					}
				}
			}
			if err == nil {
				err = svc.HandleAgentEvent(cmd.Context(), event)
			}
			if err != nil {
				if quiet {
					return nil
				}
				return fmt.Errorf("hook: %w", err)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&quiet, "quiet", false, "Suppress hook errors")
	cmd.Flags().StringVar(&oakSession, "oak-session", "", "Oak-tree session id")
	cmd.Flags().StringVar(&tmuxPane, "tmux-pane", "", "Tmux pane used to resolve the oak-tree session")
	cmd.Flags().StringVar(&eventName, "event", "", "Pi lifecycle event")
	cmd.Flags().StringVar(&cwd, "cwd", "", "Pi working directory")
	cmd.Flags().StringVar(&sessionID, "session-id", "", "Pi session id")
	cmd.Flags().StringVar(&sessionFile, "session-file", "", "Pi session file")
	cmd.Flags().StringVar(&legacyAgent, "agent", "", "Deprecated compatibility flag")
	_ = cmd.Flags().MarkHidden("agent")
	cmd.Flags().IntVar(&todoTotal, "todo-total", 0, "Visible todo count")
	cmd.Flags().IntVar(&todoPending, "todo-pending", 0, "Pending todo count")
	cmd.Flags().IntVar(&todoInProgress, "todo-in-progress", 0, "In-progress todo count")
	cmd.Flags().IntVar(&todoCompleted, "todo-completed", 0, "Completed todo count")
	cmd.Flags().StringVar(&todoJSON, "todo-json", "", "Visible todo tasks as JSON")
	cmd.Flags().StringVar(&activityKind, "activity-kind", "", "Campfire activity kind")
	cmd.Flags().StringVar(&activityMessage, "activity-message", "", "Campfire activity message")
	return cmd
}
