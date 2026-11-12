package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSkillsInstallWritesEveryBundledSkill(t *testing.T) {
	dir := t.TempDir()
	// A stale copy is overwritten, so a re-run updates the skills.
	stale := filepath.Join(dir, "using-freehire", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(stale), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stale, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := run(t, "skills", "install", "--dir", dir)
	if err != nil {
		t.Fatalf("skills install: %v\n%s", err, out)
	}
	want, _ := filepath.Glob("../../skills/*/SKILL.md")
	if len(want) == 0 {
		t.Fatal("no skills found in the repo")
	}
	for _, src := range want {
		name := filepath.Base(filepath.Dir(src))
		srcBody, _ := os.ReadFile(src)
		got, err := os.ReadFile(filepath.Join(dir, name, "SKILL.md"))
		if err != nil {
			t.Errorf("%s not installed: %v", name, err)
			continue
		}
		if string(got) != string(srcBody) {
			t.Errorf("%s: installed copy differs from the repo", name)
		}
	}
	if !strings.Contains(out, "Installed") {
		t.Errorf("output = %q", out)
	}
}
