package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// warnAPI answers a search with one ignored param, so the CLI has something to
// warn about.
func warnAPI(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"data":[{"public_slug":"go-dev","title":"Go Dev","company":"Acme"}],` +
			`"meta":{"total":42,"ignored_params":[{"param":"country","did_you_mean":"countries"}]}}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// runSplit executes the root command with stdout and stderr kept apart, so a
// test can assert that machine-readable output stays uncontaminated.
func runSplit(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	root := newRootCmd()
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), errOut.String(), err
}

func TestSearch_WarnsOnIgnoredParamsInJSONMode(t *testing.T) {
	// An agent runs with --json and pipes stdout. The warning has to reach it on
	// stderr: dropped on the floor, a widened result set looks like a real one;
	// mixed into stdout, it breaks the JSON the agent parses.
	srv := warnAPI(t)
	t.Setenv("FREEHIRE_TOKEN", "good")
	t.Setenv("FREEHIRE_API_URL", srv.URL)

	stdout, stderr, err := runSplit(t, "search", "go", "--json", "--facet", "country=it")
	if err != nil {
		t.Fatalf("search: %v", err)
	}

	if !strings.Contains(stderr, "country") || !strings.Contains(stderr, "countries") {
		t.Errorf("stderr = %q, want it to name the ignored param and the suggestion", stderr)
	}
	var parsed []map[string]any
	if err := json.Unmarshal([]byte(stdout), &parsed); err != nil {
		t.Errorf("stdout is not the JSON payload alone: %v (%q)", err, stdout)
	}
}

func TestSearch_QuietWhenNothingIgnored(t *testing.T) {
	srv := fakeAPI(t)
	t.Setenv("FREEHIRE_TOKEN", "good")
	t.Setenv("FREEHIRE_API_URL", srv.URL)

	_, stderr, err := runSplit(t, "search", "go", "--json")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if strings.TrimSpace(stderr) != "" {
		t.Errorf("stderr = %q, want silence on a clean query", stderr)
	}
}
