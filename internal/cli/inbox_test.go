package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/strelov1/freehire-cli/internal/client"
)

// TestDecodeBatchAcceptsBothShapes asserts a caller can pipe either a bare array —
// what `jq '[.[] | {...}]'` produces — or the wrapped object the API itself takes,
// so the obvious shell pipeline works without a wrapper step.
func TestDecodeBatchAcceptsBothShapes(t *testing.T) {
	bare := `[{"external_id":"<a@acme>","subject":"Hello"}]`
	wrapped := `{"messages":[{"external_id":"<a@acme>","subject":"Hello"}]}`

	for name, raw := range map[string]string{"bare array": bare, "wrapped object": wrapped} {
		t.Run(name, func(t *testing.T) {
			msgs, err := decodeBatch([]byte(raw))
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			if len(msgs) != 1 || msgs[0].ExternalID != "<a@acme>" || msgs[0].Subject != "Hello" {
				t.Errorf("decoded %+v, want one message with the id and subject", msgs)
			}
		})
	}
}

// TestDecodeBatchRejectsEmptyInput asserts a caller who pipes nothing gets an
// actionable message rather than an opaque API 400 for an empty batch.
func TestDecodeBatchRejectsEmptyInput(t *testing.T) {
	for _, raw := range []string{"", "   \n\t "} {
		if _, err := decodeBatch([]byte(raw)); err == nil {
			t.Errorf("decodeBatch(%q) succeeded, want an error naming stdin/--file", raw)
		}
	}
}

func TestDecodeBatchRejectsMalformedJSON(t *testing.T) {
	if _, err := decodeBatch([]byte(`{"messages": [`)); err == nil {
		t.Error("malformed JSON decoded without error")
	}
}

// TestParseEmailIDRejectsNonIDs asserts a mistyped id fails locally with guidance
// instead of travelling to the API and coming back an unexplained 404.
func TestParseEmailIDRejectsNonIDs(t *testing.T) {
	for _, arg := range []string{"go-dev-acme", "0", "-3", "", "12abc"} {
		if _, err := parseEmailID(arg); err == nil {
			t.Errorf("parseEmailID(%q) succeeded, want an error", arg)
		}
	}
	id, err := parseEmailID("42")
	if err != nil || id != 42 {
		t.Errorf("parseEmailID(\"42\") = %d, %v; want 42, nil", id, err)
	}
}

// inboxAPI mimics the inbox surface, recording what the CLI sent.
type inboxAPI struct {
	*httptest.Server
	lastQuery string
	lastBody  []byte
	lastPath  string
}

func newInboxAPI(t *testing.T) *inboxAPI {
	t.Helper()
	api := &inboxAPI{}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/me/inbox", func(w http.ResponseWriter, r *http.Request) {
		api.lastQuery = r.URL.RawQuery
		w.Write([]byte(`{"data":[{"id":7,"source":"external","from_name":"Acme Hiring",` +
			`"subject":"Interview invitation","status_signal":"interview_invitation",` +
			`"linked_slug":"go-dev-acme"}],"meta":{"total":1}}`))
	})
	mux.HandleFunc("/api/v1/me/emails", func(w http.ResponseWriter, r *http.Request) {
		api.lastBody, _ = readAll(r)
		w.Write([]byte(`{"data":{"inserted":2,"updated":1}}`))
	})
	mux.HandleFunc("/api/v1/me/emails/7/triage", func(w http.ResponseWriter, r *http.Request) {
		api.lastBody, _ = readAll(r)
		w.Write([]byte(`{"data":{"id":7,"status_signal":"rejection"}}`))
	})
	mux.HandleFunc("/api/v1/me/inbox/read-all", func(w http.ResponseWriter, r *http.Request) {
		api.lastQuery = r.URL.RawQuery
		w.Write([]byte(`{"data":{"marked":3}}`))
	})
	for _, action := range []string{"confirm", "reject"} {
		mux.HandleFunc("/api/v1/me/emails/7/"+action, func(w http.ResponseWriter, r *http.Request) {
			api.lastPath = r.URL.Path
			w.Write([]byte(`{"data":{"id":7,"linked_slug":"go-dev-acme","link_source":"manual"}}`))
		})
	}
	mux.HandleFunc("/api/v1/me/emails/7/application", func(w http.ResponseWriter, r *http.Request) {
		api.lastPath = r.URL.Path
		api.lastBody, _ = readAll(r)
		w.Write([]byte(`{"data":{"id":7,"linked_slug":"go-dev-acme","link_source":"manual"}}`))
	})
	api.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer good" {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"unauthorized"}`))
			return
		}
		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(api.Close)
	return api
}

func readAll(r *http.Request) ([]byte, error) {
	defer r.Body.Close()
	return io.ReadAll(r.Body)
}

// runWithStdin executes the root command with args and the given stdin,
// capturing stdout+stderr. It is `run` for the commands that read a payload.
func runWithStdin(t *testing.T, stdin string, args ...string) (string, error) {
	t.Helper()
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetIn(strings.NewReader(stdin))
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), err
}

// TestInboxListSendsTheAgentFlags asserts the work-queue flags reach the API as
// the query params it expects — the pairing the agent loop depends on.
func TestInboxListSendsTheAgentFlags(t *testing.T) {
	api := newInboxAPI(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FREEHIRE_TOKEN", "good")

	out, err := run(t, "inbox", "list", "--unclassified", "--body", "--source", "external", "--api-url", api.URL)
	if err != nil {
		t.Fatalf("inbox list: %v", err)
	}
	for _, want := range []string{"unclassified=1", "body=1", "source=external"} {
		if !strings.Contains(api.lastQuery, want) {
			t.Errorf("query %q missing %q", api.lastQuery, want)
		}
	}
	// The row shows the triage state so an agent (or a human) can see what is done.
	if !strings.Contains(out, "interview_invitation") || !strings.Contains(out, "go-dev-acme") {
		t.Errorf("listing output = %q, want it to show the signal and linked slug", out)
	}
}

// TestInboxPushReadsStdin asserts the documented pipeline — a mail client's JSON
// piped straight in — reaches the API as a wrapped batch.
func TestInboxPushReadsStdin(t *testing.T) {
	api := newInboxAPI(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FREEHIRE_TOKEN", "good")

	out, err := runWithStdin(t, `[{"external_id":"<a@acme>","subject":"Hi"}]`,
		"inbox", "push", "--api-url", api.URL)
	if err != nil {
		t.Fatalf("inbox push: %v", err)
	}

	var sent struct {
		Messages []client.IngestMessage `json:"messages"`
	}
	if err := json.Unmarshal(api.lastBody, &sent); err != nil {
		t.Fatalf("decode what the CLI sent (%q): %v", api.lastBody, err)
	}
	if len(sent.Messages) != 1 || sent.Messages[0].ExternalID != "<a@acme>" {
		t.Errorf("CLI sent %+v, want the piped message", sent.Messages)
	}
	if !strings.Contains(out, "2 new, 1 updated") {
		t.Errorf("push output = %q, want the inserted/updated counts", out)
	}
}

// TestInboxTriageSendsSignalAndSlug asserts the verdict travels whole, and that
// confidence is omitted rather than sent as a misleading zero when not given.
func TestInboxTriageSendsSignalAndSlug(t *testing.T) {
	api := newInboxAPI(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FREEHIRE_TOKEN", "good")

	if _, err := run(t, "inbox", "triage", "7", "rejection", "--slug", "go-dev-acme", "--api-url", api.URL); err != nil {
		t.Fatalf("triage: %v", err)
	}
	body := string(api.lastBody)
	if !strings.Contains(body, `"signal":"rejection"`) || !strings.Contains(body, `"slug":"go-dev-acme"`) {
		t.Errorf("triage body = %q, want the signal and slug", body)
	}
	if strings.Contains(body, "confidence") {
		t.Errorf("triage body = %q, want confidence omitted when not passed", body)
	}
}
