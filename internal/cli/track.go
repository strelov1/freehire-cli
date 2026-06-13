package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/strelov1/freehire-cli/internal/client"
)

// stageVocabulary lists the known application stages, in pipeline order, for the
// `stage` command's help. The API validates the value (it is the source of
// truth), so an unknown stage still reaches the server and surfaces its error —
// this list is guidance only and cannot drift the enforcement.
var stageVocabulary = []string{
	"applied", "screening", "responded", "interview", "offer",
	"accepted", "rejected", "withdrawn",
}

func newStageCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stage <slug> <stage>",
		Short: "Set a job's application stage",
		Long:  "Set the application stage for a tracked job.\n\nStages: " + strings.Join(stageVocabulary, ", "),
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _, err := authedClient(cmd)
			if err != nil {
				return err
			}
			slug, stage := args[0], args[1]
			data, err := c.Track(cmd.Context(), slug, client.TrackParams{Stage: &stage})
			if err != nil {
				return err
			}
			if wantJSON(cmd) {
				printJSON(cmd, data)
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Stage set: %s → %s\n", slug, stage)
			return nil
		},
	}
}

func newNoteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "note <slug> <text>...",
		Short: "Attach a note to a tracked job",
		Long: "Attach a free-text note to a tracked job. The note is the trailing " +
			"arguments joined with spaces, so it need not be quoted.",
		Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _, err := authedClient(cmd)
			if err != nil {
				return err
			}
			slug := args[0]
			note := strings.Join(args[1:], " ")
			data, err := c.Track(cmd.Context(), slug, client.TrackParams{Notes: &note})
			if err != nil {
				return err
			}
			if wantJSON(cmd) {
				printJSON(cmd, data)
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Note saved: %s\n", slug)
			return nil
		},
	}
}
