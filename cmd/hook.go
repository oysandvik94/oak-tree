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
	cmd.AddCommand(newAgentEventHookCommand())
	return cmd
}

func newAgentEventHookCommand() *cobra.Command {
	var quiet bool
	var oakSession, eventName, cwd, sessionID, sessionFile, todoJSON, legacyAgent string
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
				event.OakSessionID, event.Event, event.Cwd, event.SessionID, event.SessionFile = oakSession, eventName, cwd, sessionID, sessionFile
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
	return cmd
}
