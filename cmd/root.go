package cmd

import (
	"context"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"

	"github.com/oysandvik94/oak-tree/internal/oaktree"
)

var rootCmd = &cobra.Command{
	Use:          "oak-tree",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDashboard(cmd.Context(), os.Stdout, os.Stdin)
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(newNewCommand(), newCloseCommand(), newHookCommand(), newPopupCommand())
}

func newService() (*oaktree.Service, error) {
	paths, err := oaktree.DefaultPaths("")
	if err != nil {
		return nil, err
	}
	store := oaktree.NewStore(paths.StateDir)
	return oaktree.NewService(paths, store, oaktree.NewOSRunner(paths.StateDir)), nil
}

func runDashboard(ctx context.Context, stdout *os.File, stdin *os.File) error {
	svc, err := newService()
	if err != nil {
		return err
	}
	cfg, err := oaktree.LoadConfig()
	if err != nil {
		return err
	}
	model := oaktree.NewDashboardModel(svc, cfg)
	p := tea.NewProgram(model, tea.WithContext(ctx))
	_, err = p.Run()
	return err
}
