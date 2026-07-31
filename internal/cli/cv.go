package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
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
		Short: "Edit a CV by path",
		Long: "Edit a CV. An edit is a kind (set, insert, remove, move) and a path into the " +
			"document — `summary`, `experience[2].bullets[1]`, `skills[0].items[3]`, " +
			"`education[1].degree`, `style.font_size`. Indices are 0-based.\n\n" +
			"One edit fits on the command line:\n" +
			"  freehire cv edit <cv-id> --set 'experience[0].bullets[1]=Cut p99 latency 40%'\n" +
			"  freehire cv edit <cv-id> --remove 'experience[0].bullets[2]'\n\n" +
			"Several go in as JSON, via --ops or on stdin:\n" +
			`  --ops '[{"kind":"set","path":"summary","value":"…"}]'` + "\n\n" +
			"The whole batch applies or none of it does: an unknown path or an index past the " +
			"end is a 422 and the CV is untouched. Editing with an API key edits as the " +
			"tailoring agent, so the candidate's own header fields are refused, and anything " +
			"stating what they did needs --evidence (the id of a banked achievement).",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := cvID(args[0])
			if err != nil {
				return err
			}
			body, err := readEdits(cmd)
			if err != nil {
				return err
			}
			c, _, err := authedClient(cmd)
			if err != nil {
				return err
			}
			data, err := c.EditCV(cmd.Context(), id, body)
			if err != nil {
				return err
			}
			if wantJSON(cmd) {
				printJSON(cmd, data)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "CV %s updated\n", id)
			}
			return nil
		},
	}
	cmd.Flags().String("set", "", "one edit as path=value, e.g. 'experience[0].bullets[1]=Cut p99 latency 40%'")
	cmd.Flags().String("insert", "", "insert at a position, as path=value")
	cmd.Flags().String("remove", "", "remove what is at a path")
	cmd.Flags().String("evidence", "", "the banked achievement an edit rests on (required for a claim about the candidate)")
	cmd.Flags().String("note", "", "one line on why, shown beside the edit in the CV's history")
	cmd.Flags().String("ops", "", "the edits as JSON (read from stdin when no flag is given)")
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
				out = fmt.Sprintf("cv-%s.pdf", id)
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

// cvID reads the positional CV id argument. The id is opaque — the API hands it
// out and the CLI only passes it back — so there is deliberately no format check
// here beyond "not empty": validating the shape would bake today's format into a
// released binary, which is exactly what an opaque id is meant to avoid. A wrong
// id comes back from the server as a 404.
func cvID(arg string) (string, error) {
	id := strings.TrimSpace(arg)
	if id == "" {
		return "", fmt.Errorf("a cv id is required — copy it from `freehire cv` or the tailoring workspace URL")
	}
	return id, nil
}

// readEdits builds the request body for `cv edit`.
//
// The shorthand flags cover the common case — one edit, on one line — and JSON covers the
// rest. They are deliberately not mixable: a command that took both would have to decide
// which order they apply in, and the answer would only ever be guessed at.
//
// JSON is accepted in three shapes, because all three are things a caller reasonably has in
// hand: the whole body, a bare array of edits, or a single edit object.
func readEdits(cmd *cobra.Command) (json.RawMessage, error) {
	note, _ := cmd.Flags().GetString("note")
	if op, ok, err := shorthandEdit(cmd); err != nil {
		return nil, err
	} else if ok {
		return json.Marshal(editRequest{Ops: []editOp{op}, Note: note})
	}

	raw, _ := cmd.Flags().GetString("ops")
	if strings.TrimSpace(raw) == "" {
		b, err := io.ReadAll(cmd.InOrStdin())
		if err != nil {
			return nil, err
		}
		raw = string(b)
	}
	if strings.TrimSpace(raw) == "" {
		return nil, fmt.Errorf("no edit provided: pass --set 'path=value', --remove 'path', " +
			"--ops '<json>', or pipe the JSON on stdin")
	}
	return normalizeEdits(raw, note)
}

// editOp mirrors one operation on the wire.
type editOp struct {
	Kind       string `json:"kind"`
	Path       string `json:"path"`
	Value      any    `json:"value,omitempty"`
	EvidenceID string `json:"evidence_id,omitempty"`
}

type editRequest struct {
	Ops  []editOp `json:"ops"`
	Note string   `json:"note,omitempty"`
}

// shorthandEdit reads --set / --insert / --remove. Exactly one may be given.
func shorthandEdit(cmd *cobra.Command) (editOp, bool, error) {
	evidence, _ := cmd.Flags().GetString("evidence")
	var (
		found editOp
		count int
	)
	for kind, flag := range map[string]string{"set": "set", "insert": "insert", "remove": "remove"} {
		v, _ := cmd.Flags().GetString(flag)
		if strings.TrimSpace(v) == "" {
			continue
		}
		count++
		op := editOp{Kind: kind, EvidenceID: evidence}
		if kind == "remove" {
			op.Path = strings.TrimSpace(v)
		} else {
			path, value, ok := strings.Cut(v, "=")
			if !ok {
				return editOp{}, false, fmt.Errorf("--%s takes path=value, e.g. --%s 'summary=Ten years of Go'", flag, flag)
			}
			op.Path, op.Value = strings.TrimSpace(path), value
		}
		found = op
	}
	if count > 1 {
		return editOp{}, false, fmt.Errorf("pass one of --set, --insert or --remove; for several edits use --ops '<json>'")
	}
	return found, count == 1, nil
}

// normalizeEdits accepts the whole body, a bare array of edits, or a single edit, and
// returns the body the API expects.
func normalizeEdits(raw, note string) (json.RawMessage, error) {
	trimmed := strings.TrimSpace(raw)
	switch {
	case strings.HasPrefix(trimmed, "["):
		var ops []json.RawMessage
		if err := json.Unmarshal([]byte(trimmed), &ops); err != nil {
			return nil, fmt.Errorf("the edits are not valid JSON: %w", err)
		}
		return json.Marshal(map[string]any{"ops": ops, "note": note})
	case strings.Contains(trimmed, `"ops"`):
		if note == "" {
			return json.RawMessage(trimmed), nil
		}
		var body map[string]any
		if err := json.Unmarshal([]byte(trimmed), &body); err != nil {
			return nil, fmt.Errorf("the edits are not valid JSON: %w", err)
		}
		body["note"] = note
		return json.Marshal(body)
	default:
		var one json.RawMessage
		if err := json.Unmarshal([]byte(trimmed), &one); err != nil {
			return nil, fmt.Errorf("the edit is not valid JSON: %w", err)
		}
		return json.Marshal(map[string]any{"ops": []json.RawMessage{one}, "note": note})
	}
}
