package cli

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// `freehire mcp` relays stdio to the hosted endpoint at the configured API URL, with the
// stored credential and the CLI's User-Agent, which is how the server tells the session
// it has a shell.
func TestMCPRelaysStdioToTheHostedEndpoint(t *testing.T) {
	var path, agent, auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, agent, auth = r.URL.Path, r.Header.Get("User-Agent"), r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2025-06-18"}}`))
	}))
	defer srv.Close()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FREEHIRE_TOKEN", "good")
	t.Setenv("FREEHIRE_API_URL", srv.URL)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetIn(strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}` + "\n"))
	root.SetArgs([]string{"mcp"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if path != "/api/v1/mcp" || !strings.HasPrefix(agent, "freehire-cli/") || auth != "Bearer good" {
		t.Fatalf("server saw path %q, User-Agent %q, Authorization %q", path, agent, auth)
	}
	if want := `{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2025-06-18"}}` + "\n"; out.String() != want {
		t.Fatalf("stdout = %q, want %q", out.String(), want)
	}
}
