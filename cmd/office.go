package cmd

import (
	"context"
	"errors"
	"fmt"
	"os/signal"
	"syscall"

	"github.com/oysandvik94/oak-tree/internal/oaktree"
	"github.com/spf13/cobra"
)

func newOfficeCommand() *cobra.Command {
	var serve bool
	var stateDir string
	command := &cobra.Command{
		Use:   "office",
		Short: "Open the local browser office",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if serve {
				if stateDir == "" {
					return errors.New("office server requires a state directory")
				}
				ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
				defer stop()
				return oaktree.ServeOffice(ctx, oaktree.NewStore(stateDir))
			}
			if stateDir != "" {
				return errors.New("office state directory is only valid for the server")
			}
			svc, err := newService()
			if err != nil {
				return err
			}
			if _, err := oaktree.EnsurePiExtension(svc.Paths); err != nil {
				return fmt.Errorf("prepare Pi extension: %w", err)
			}
			return oaktree.OpenOffice(cmd.Context(), svc.Paths.StateDir)
		},
	}
	command.Flags().BoolVar(&serve, "serve", false, "run the local office server")
	_ = command.Flags().MarkHidden("serve")
	command.Flags().StringVar(&stateDir, "state-dir", "", "server state directory")
	_ = command.Flags().MarkHidden("state-dir")
	return command
}
