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
		Long: "Run a whole tailoring cycle. `tailor` starts one for a vacancy and hands back the " +
			"CV id every other command here takes; `list` shows the tailored CVs you already " +
			"have. Then `context` shows the fit analysis to reframe toward, `get` dumps the CV " +
			"document, `edit` changes it by path, and `render` downloads the PDF. Every edit is " +
			"recorded and can be undone on its own from the tailoring workspace.",
	}
	cv.AddCommand(newCVListCmd(), newCVTailorCmd(), newCVContextCmd(), newCVGetCmd(),
		newCVEditCmd(), newCVRenderCmd())
	return cv
}

// cvListItem is the subset of a tailored CV shown in a `cv list` row. The id leads because
// it is what the reader came for — every other command in the group is addressed by one.
type cvListItem struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	JobSlug    string `json:"job_slug"`
	JobTitle   string `json:"job_title"`
	JobCompany string `json:"job_company"`
	UpdatedAt  string `json:"updated_at"`
}

func newCVListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List your tailored CVs",
		Long: "Print the CVs you have tailored to a vacancy, newest edit first, with the CV id " +
			"and the vacancy each was written for. The id is what `context`, `get`, `edit` and " +
			"`render` take — copy it from here rather than constructing one.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, _, err := authedClient(cmd)
			if err != nil {
				return err
			}
			data, err := c.ListCVs(cmd.Context())
			if err != nil {
				return err
			}
			if wantJSON(cmd) {
				printJSON(cmd, data)
				return nil
			}
			var items []cvListItem
			if err := json.Unmarshal(data, &items); err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if len(items) == 0 {
				fmt.Fprintln(out, "No tailored CVs yet.")
				fmt.Fprintln(out, "Start one with: freehire cv tailor <job-slug>")
				return nil
			}
			for _, it := range items {
				fmt.Fprintf(out, "%s  %s\n", it.ID, cvListVacancy(it))
			}
			return nil
		},
	}
}

// cvListVacancy renders the vacancy a tailored CV belongs to. It falls back through what is
// actually populated: a copy made before the job columns were denormalised carries only the
// slug, and one whose vacancy has since been pruned carries only its own title. Printing an
// empty column for either would make the row look broken when it is merely older.
func cvListVacancy(it cvListItem) string {
	if it.JobTitle != "" && it.JobCompany != "" {
		return fmt.Sprintf("%s at %s (%s)", it.JobTitle, it.JobCompany, it.JobSlug)
	}
	for _, v := range []string{it.JobTitle, it.JobSlug, it.Title} {
		if v != "" {
			return v
		}
	}
	return "(untitled)"
}

// cvTailorResult is the bootstrap payload: the ids and the conversation the workspace resumes.
type cvTailorResult struct {
	TailorCVID string `json:"tailor_cv_id"`
	BaseCVID   string `json:"base_cv_id"`
	SessionID  string `json:"session_id"`
	// ColdStartRunning is true only on the call that CREATED the copy, which is how the
	// human output can say "started" or "reopened" instead of guessing.
	ColdStartRunning bool `json:"cold_start_running"`
}

func newCVTailorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tailor <job-slug>",
		Short: "Start tailoring a CV to a vacancy",
		Long: "Create a copy of your base CV bound to a vacancy, and print the CV id the rest of " +
			"this group takes. Idempotent per vacancy: running it again for the same slug " +
			"reopens the copy that already exists rather than making a second one.\n\n" +
			"It spends AI credits the first time it creates the copy (a 402 means the balance " +
			"will not cover it) and needs a résumé to seed a base CV from (409 when there is " +
			"none). It does not call a model — the reframing is the agent's work, done through " +
			"`cv context` and `cv edit`.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			slug := strings.TrimSpace(args[0])
			if slug == "" {
				return fmt.Errorf("a job slug is required, e.g. `freehire cv tailor go-developer-acme-1a2b3c4d`")
			}
			c, _, err := authedClient(cmd)
			if err != nil {
				return err
			}
			data, err := c.TailorCV(cmd.Context(), slug)
			if err != nil {
				return err
			}
			if wantJSON(cmd) {
				printJSON(cmd, data)
				return nil
			}
			var r cvTailorResult
			if err := json.Unmarshal(data, &r); err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			verb := "Reopened"
			if r.ColdStartRunning {
				verb = "Started"
			}
			fmt.Fprintf(out, "%s tailoring for %s\n", verb, slug)
			fmt.Fprintf(out, "  tailored CV: %s\n", r.TailorCVID)
			fmt.Fprintf(out, "  base CV:     %s\n", r.BaseCVID)
			fmt.Fprintf(out, "\nNext: freehire cv context %s\n", r.TailorCVID)
			return nil
		},
	}
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
		return "", fmt.Errorf("a cv id is required — copy it from `freehire cv list`, or start one with `freehire cv tailor <job-slug>`")
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
