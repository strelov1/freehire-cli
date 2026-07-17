package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// newCVCmd is the `cv` command group for interactive CV tailoring. The endpoints are
// beta-gated on the server and act as the authenticated user; the tailoring agent drives
// them with its minted session key. Commands are addressed by CV id (from the tailoring
// bootstrap), not slug.
func newCVCmd() *cobra.Command {
	cv := &cobra.Command{
		Use:   "cv",
		Short: "Tailor a CV to a vacancy (beta)",
		Long: "Read and edit a tailored CV during a tailoring session. `context` shows the fit " +
			"analysis to reframe toward, `get` dumps the CV document, `edit` applies one " +
			"field-level patch, and `render` downloads the PDF. Addressed by CV id.",
	}
	cv.AddCommand(newCVContextCmd(), newCVGetCmd(), newCVEditCmd(), newCVRenderCmd())
	return cv
}

func newCVContextCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "context <cv-id>",
		Short: "Show the fit-analysis context for a tailored CV",
		Long: "Print the cached fit analysis a tailored CV should reframe toward: the verdict, " +
			"recommendation, dimension comments, and the requirement split — missing_have " +
			"(reframe existing evidence) vs missing_gap (ask the candidate before adding). " +
			"Output is JSON.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := cvID(args[0])
			if err != nil {
				return err
			}
			c, _, err := authedClient(cmd)
			if err != nil {
				return err
			}
			data, err := c.TailorCVContext(cmd.Context(), id)
			if err != nil {
				return err
			}
			printJSON(cmd, data)
			return nil
		},
	}
}

func newCVGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <cv-id>",
		Short: "Print a CV with its full document",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := cvID(args[0])
			if err != nil {
				return err
			}
			c, _, err := authedClient(cmd)
			if err != nil {
				return err
			}
			data, err := c.GetCV(cmd.Context(), id)
			if err != nil {
				return err
			}
			printJSON(cmd, data)
			return nil
		},
	}
}

func newCVEditCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "edit <cv-id>",
		Short: "Apply one field-level patch to a CV",
		Long: "Apply a single field-level patch to a CV. The patch is a cv.Patch JSON object " +
			"passed via --patch or on stdin, e.g. " +
			`'{"op":"add_bullet","experience":0,"value":"Led the migration"}'. Ops: ` +
			"set_summary, set_header_field, add_bullet, replace_bullet, remove_bullet, " +
			"reorder_bullets, set_skill_group. The server sanitizes and validates it (a bad " +
			"patch is a 422).",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := cvID(args[0])
			if err != nil {
				return err
			}
			patch, err := readPatch(cmd)
			if err != nil {
				return err
			}
			c, _, err := authedClient(cmd)
			if err != nil {
				return err
			}
			data, err := c.PatchCV(cmd.Context(), id, patch)
			if err != nil {
				return err
			}
			if wantJSON(cmd) {
				printJSON(cmd, data)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "CV %d updated\n", id)
			}
			return nil
		},
	}
	cmd.Flags().String("patch", "", "the cv.Patch JSON (read from stdin when omitted)")
	return cmd
}

func newCVRenderCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "render <cv-id>",
		Short: "Download a CV rendered to PDF",
		Long: "Download the ATS PDF render of a CV. Writes to --out (default cv-<id>.pdf), or to " +
			"stdout when --out is \"-\".",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := cvID(args[0])
			if err != nil {
				return err
			}
			c, _, err := authedClient(cmd)
			if err != nil {
				return err
			}
			pdf, err := c.RenderCV(cmd.Context(), id)
			if err != nil {
				return err
			}
			out, _ := cmd.Flags().GetString("out")
			if out == "-" {
				_, err := cmd.OutOrStdout().Write(pdf)
				return err
			}
			if out == "" {
				out = fmt.Sprintf("cv-%d.pdf", id)
			}
			if err := os.WriteFile(out, pdf, 0o644); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "wrote %s (%d bytes)\n", out, len(pdf))
			return nil
		},
	}
	cmd.Flags().String("out", "", `output path for the PDF (default cv-<id>.pdf; "-" for stdout)`)
	return cmd
}

// cvID parses the positional CV id argument.
func cvID(arg string) (int64, error) {
	id, err := strconv.ParseInt(arg, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid cv id %q: must be a number", arg)
	}
	return id, nil
}

// readPatch returns the patch JSON from --patch, or reads it from stdin when the flag is
// absent. It errors on an empty patch rather than sending a no-op to the server.
func readPatch(cmd *cobra.Command) (json.RawMessage, error) {
	if p, _ := cmd.Flags().GetString("patch"); strings.TrimSpace(p) != "" {
		return json.RawMessage(p), nil
	}
	b, err := io.ReadAll(cmd.InOrStdin())
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(string(b)) == "" {
		return nil, fmt.Errorf("no patch provided: pass --patch '<json>' or pipe it on stdin")
	}
	return json.RawMessage(b), nil
}
