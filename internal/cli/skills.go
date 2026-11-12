package cli

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/strelov1/freehire-cli/skills"
)

func newSkillsCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "skills", Short: "Manage the freehire agent skills"}
	cmd.AddCommand(newSkillsInstallCmd())
	return cmd
}

func newSkillsInstallCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Copy the freehire agent skills into ~/.claude/skills (or --dir)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			dir, _ := cmd.Flags().GetString("dir")
			if dir == "" {
				home, err := os.UserHomeDir()
				if err != nil {
					return err
				}
				dir = filepath.Join(home, ".claude", "skills")
			}
			names, err := installSkills(dir)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			for _, n := range names {
				fmt.Fprintf(out, "  %s\n", n)
			}
			fmt.Fprintf(out, "Installed %d skills into %s. Restart your agent to load them.\n", len(names), dir)
			return nil
		},
	}
	cmd.Flags().String("dir", "", "skills directory (default ~/.claude/skills); point it at another agent's skills folder")
	return cmd
}

// installSkills writes every bundled skill under dir, overwriting an older copy
// of the same skill so a re-run updates them. It returns the skill names.
func installSkills(dir string) ([]string, error) {
	entries, err := fs.ReadDir(skills.FS, ".")
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		body, err := fs.ReadFile(skills.FS, e.Name()+"/SKILL.md")
		if err != nil {
			return nil, err
		}
		target := filepath.Join(dir, e.Name())
		if err := os.MkdirAll(target, 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(filepath.Join(target, "SKILL.md"), body, 0o644); err != nil {
			return nil, err
		}
		names = append(names, e.Name())
	}
	return names, nil
}
