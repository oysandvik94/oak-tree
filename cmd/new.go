package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/oysandvik94/oak-tree/internal/oaktree"
)

func newNewCommand() *cobra.Command {
	var root string
	var branch string

	cmd := &cobra.Command{
		Use:          "new",
		SilenceUsage: true,
		Short:        "Create a new oak-tree session",
		RunE: func(cmd *cobra.Command, args []string) error {
			if root == "" {
				return fmt.Errorf("--root is required")
			}
			svc, err := newService()
			if err != nil {
				return err
			}
			session, err := svc.CreateSession(cmd.Context(), oaktree.CreateSessionInput{
				Root:   root,
				Branch: branch,
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\n", session.ID, session.Workdir)
			return nil
		},
	}
	cmd.Flags().StringVar(&root, "root", "", "Repository root directory")
	cmd.Flags().StringVar(&branch, "branch", "", "Optional branch name")
	return cmd
}
