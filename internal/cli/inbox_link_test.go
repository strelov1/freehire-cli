package cli

import (
	"strings"
	"testing"
)

// TestInboxListSendsTheLinkFilter asserts the link-state filter reaches the API
// as the query param it expects. `--link suggested` is the confirmation queue and
// `--link unlinked` the mail with no application to attach to; without them an
// agent has to page the whole mailbox and sort it itself.
func TestInboxListSendsTheLinkFilter(t *testing.T) {
	api := newInboxAPI(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FREEHIRE_TOKEN", "good")

	for _, state := range []string{"linked", "suggested", "unlinked"} {
		if _, err := run(t, "inbox", "list", "--link", state, "--api-url", api.URL); err != nil {
			t.Fatalf("inbox list --link %s: %v", state, err)
		}
		if !strings.Contains(api.lastQuery, "link="+state) {
			t.Errorf("query %q missing link=%s", api.lastQuery, state)
		}
	}
}

// TestInboxListRejectsAnUnknownLinkState asserts a typo fails locally with the
// vocabulary in the message, rather than travelling to the API and returning an
// unexplained 400.
func TestInboxListRejectsAnUnknownLinkState(t *testing.T) {
	api := newInboxAPI(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FREEHIRE_TOKEN", "good")

	out, err := run(t, "inbox", "list", "--link", "bogus", "--api-url", api.URL)
	if err == nil {
		t.Fatal("--link bogus succeeded, want a local error")
	}
	if !strings.Contains(err.Error()+out, "suggested") {
		t.Errorf("error %q should name the accepted values", err)
	}
}

// TestInboxReadAllSendsTheLinkFilter asserts mark-all-read carries the same
// filter the listing did. If it did not, clearing a queue of three would mark the
// whole mailbox read.
func TestInboxReadAllSendsTheLinkFilter(t *testing.T) {
	api := newInboxAPI(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FREEHIRE_TOKEN", "good")

	if _, err := run(t, "inbox", "read-all", "--link", "suggested", "--api-url", api.URL); err != nil {
		t.Fatalf("inbox read-all: %v", err)
	}
	if !strings.Contains(api.lastQuery, "link=suggested") {
		t.Errorf("read-all query %q missing link=suggested", api.lastQuery)
	}
}

// TestInboxConfirmAndReject asserts the two suggestion verdicts reach their own
// endpoints. These are what drain the suggestion queue the matcher fills.
func TestInboxConfirmAndReject(t *testing.T) {
	api := newInboxAPI(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FREEHIRE_TOKEN", "good")

	for _, action := range []string{"confirm", "reject"} {
		if _, err := run(t, "inbox", action, "7", "--api-url", api.URL); err != nil {
			t.Fatalf("inbox %s: %v", action, err)
		}
		if want := "/api/v1/me/emails/7/" + action; api.lastPath != want {
			t.Errorf("inbox %s hit %q, want %q", action, api.lastPath, want)
		}
	}
}

// TestInboxApplicationSendsTheSlug asserts create-and-link posts the job slug to
// the application endpoint — the path for mail about an application that was
// never recorded.
func TestInboxApplicationSendsTheSlug(t *testing.T) {
	api := newInboxAPI(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FREEHIRE_TOKEN", "good")

	out, err := run(t, "inbox", "application", "7", "go-dev-acme", "--api-url", api.URL)
	if err != nil {
		t.Fatalf("inbox application: %v", err)
	}
	if api.lastPath != "/api/v1/me/emails/7/application" {
		t.Errorf("hit %q, want the application endpoint", api.lastPath)
	}
	if body := string(api.lastBody); !strings.Contains(body, `"slug":"go-dev-acme"`) {
		t.Errorf("body = %q, want the slug", body)
	}
	if !strings.Contains(out, "go-dev-acme") {
		t.Errorf("output = %q, want it to name the application", out)
	}
}
