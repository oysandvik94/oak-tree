package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"

	"github.com/oysandvik94/oak-tree/internal/oaktree"
)

func newPopupCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "popup",
		SilenceUsage: true,
		Short:        "Open the dashboard in a tmux popup when available",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := newService()
			if err != nil {
				return err
			}
			return runPopup(cmd.Context(), svc)
		},
	}
	return cmd
}

func runPopup(ctx context.Context, svc *oaktree.Service) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if os.Getenv("TMUX") != "" {
		command := exec.CommandContext(ctx, "tmux", "display-popup", "-E", "-w", "90%", "-h", "90%", "--", exe)
		command.Stdout = os.Stdout
		command.Stderr = os.Stderr
		command.Stdin = os.Stdin
		if err := command.Run(); err == nil {
			return nil
		}
	}
	cfg, err := oaktree.LoadConfig()
	if err != nil {
		return err
	}
	model := oaktree.NewDashboardModel(svc, cfg)
	p := tea.NewProgram(model, tea.WithContext(ctx))
	_, err = p.Run()
	if err != nil {
		return fmt.Errorf("popup dashboard: %w", err)
	}
	return nil
}
