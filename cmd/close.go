package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newCloseCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "close SESSION_ID",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		Short:        "Close an oak-tree session",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := newService()
			if err != nil {
				return err
			}
			if err := svc.CloseSession(cmd.Context(), args[0]); err != nil {
				return fmt.Errorf("close %s: %w", args[0], err)
			}
			return nil
		},
	}
	return cmd
}
