package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// semanticAPI answers every search with the given meta.semantic and records the query.
func semanticAPI(t *testing.T, semantic string) (*httptest.Server, *url.Values) {
	t.Helper()
	var got url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Query()
		w.Write([]byte(`{"data":[{"public_slug":"reading-x","title":"Reading Interventionist","company":"Acme"}],` +
			`"meta":{"total":1,"semantic":"` + semantic + `"}}`))
	}))
	t.Cleanup(srv.Close)
	return srv, &got
}

func TestSearch_SemanticFlagReachesTheAPI(t *testing.T) {
	srv, got := semanticAPI(t, "applied")
	t.Setenv("FREEHIRE_API_URL", srv.URL)

	_, stderr, err := runSplit(t, "search", "help kids learn to read", "--semantic")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if got.Get("semantic") != "true" || got.Get("q") != "help kids learn to read" {
		t.Errorf("query = %v, want semantic=true with the text", *got)
	}
	if strings.TrimSpace(stderr) != "" {
		t.Errorf("stderr = %q, want silence when the semantic search ran", stderr)
	}
}

func TestSearch_WarnsWhenSemanticFellBackToKeywords(t *testing.T) {
	// Same rule as an ignored filter: the warning goes to stderr, so --json stdout stays
	// parseable while the agent still learns the answer is keyword-only.
	srv, _ := semanticAPI(t, "unavailable")
	t.Setenv("FREEHIRE_API_URL", srv.URL)

	stdout, stderr, err := runSplit(t, "search", "help kids learn to read", "--semantic", "--json")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if !strings.Contains(stderr, "semantic") || !strings.Contains(stderr, "keyword") {
		t.Errorf("stderr = %q, want a warning that the results are keyword-only", stderr)
	}
	var parsed []map[string]any
	if err := json.Unmarshal([]byte(stdout), &parsed); err != nil {
		t.Errorf("stdout is not the JSON payload alone: %v (%q)", err, stdout)
	}
}
