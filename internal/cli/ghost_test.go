package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ghostServer answers the report channel, recording what reached it.
type ghostServer struct {
	*httptest.Server
	calls  int
	method string
	body   map[string]any
}

func newGhostServer(t *testing.T, status int) *ghostServer {
	t.Helper()
	g := &ghostServer{}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/jobs/go-dev-acme/ghost-report", func(w http.ResponseWriter, r *http.Request) {
		g.calls++
		g.method = r.Method
		_ = json.NewDecoder(r.Body).Decode(&g.body)
		w.WriteHeader(status)
		if status == http.StatusCreated {
			_, _ = w.Write([]byte(`{"data":{"job_id":1,"applied_on":"2026-06-01"}}`))
		}
	})
	g.Server = httptest.NewServer(mux)
	t.Cleanup(g.Close)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FREEHIRE_TOKEN", "good")
	return g
}

func TestGhostReportFilesTheStatedDay(t *testing.T) {
	srv := newGhostServer(t, http.StatusCreated)

	out, err := run(t, "ghost", "report", "go-dev-acme", "--applied-on", "2026-06-01", "--api-url", srv.URL)
	if err != nil {
		t.Fatalf("ghost report: %v", err)
	}
	if srv.method != http.MethodPost {
		t.Errorf("method = %s, want POST", srv.method)
	}
	if srv.body["applied_on"] != "2026-06-01" {
		t.Errorf("applied_on = %v, want the stated day", srv.body["applied_on"])
	}
	if !strings.Contains(out, "2026-06-01") {
		t.Errorf("output = %q, want it to echo the date it filed", out)
	}
}

// The date is the substance of the claim, so a missing one must fail before any
// request — an agent defaulting to today would file a date the person never stated.
func TestGhostReportRefusesToInventTheDate(t *testing.T) {
	srv := newGhostServer(t, http.StatusCreated)

	if _, err := run(t, "ghost", "report", "go-dev-acme", "--api-url", srv.URL); err == nil {
		t.Error("ghost report without --applied-on should fail")
	}
	if _, err := run(t, "ghost", "report", "go-dev-acme", "--applied-on", "last tuesday", "--api-url", srv.URL); err == nil {
		t.Error("ghost report with an unparseable date should fail")
	}
	if srv.calls != 0 {
		t.Errorf("calls = %d, want 0 — neither claim should have reached the server", srv.calls)
	}
}

// The API answers 204 with no body. A client that insisted on decoding one would
// report a failure on a call that in fact succeeded.
func TestGhostRetractAcceptsAnEmptyAnswer(t *testing.T) {
	srv := newGhostServer(t, http.StatusNoContent)

	out, err := run(t, "ghost", "retract", "go-dev-acme", "--api-url", srv.URL)
	if err != nil {
		t.Fatalf("ghost retract: %v", err)
	}
	if srv.method != http.MethodDelete {
		t.Errorf("method = %s, want DELETE", srv.method)
	}
	if !strings.Contains(out, "Withdrawn") {
		t.Errorf("output = %q, want it to confirm the withdrawal", out)
	}
}

func TestJobShowsTheSignalAboveTheDescription(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/jobs/go-dev-acme", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"public_slug":"go-dev-acme","title":"Go Dev","company":"Acme",
			"location":"Remote","url":"https://acme.test/go","description":"We are hiring.",
			"ghost":{"level":"possible","criteria":["evergreen_posting","ats_absent"],
			"criteria_total":4,"ats_checked_at":"2026-07-30T09:00:00Z"}}}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FREEHIRE_TOKEN", "good")

	out, err := run(t, "job", "go-dev-acme", "--api-url", srv.URL)
	if err != nil {
		t.Fatalf("job: %v", err)
	}
	for _, want := range []string{
		"Possibly inactive — 2 of 4 checks fired",
		"Not on the company's own careers board (checked 2026-07-30)",
		"Not observed: applications here, reports from people.",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output = %q, want it to contain %q", out, want)
		}
	}
	if strings.Index(out, "Possibly inactive") > strings.Index(out, "We are hiring.") {
		t.Error("the signal printed below the description; a reader would never reach it")
	}
}

func TestGhostSignalLines(t *testing.T) {
	three := 3
	cases := []struct {
		name       string
		in         *ghostSignal
		wantEmpty  bool
		wantSubstr string
		wantAbsent string
	}{
		{
			name:      "no signal at all",
			in:        nil,
			wantEmpty: true,
		},
		{
			// Most of the catalogue: criteria recorded, but not enough converged.
			name:      "level none says nothing",
			in:        &ghostSignal{Level: "none", Criteria: []string{"ats_absent"}, CriteriaTotal: 4},
			wantEmpty: true,
		},
		{
			// A level this build cannot word must render nothing rather than a bare scale.
			name:      "a level from a newer server",
			in:        &ghostSignal{Level: "certain", Criteria: []string{"ats_absent"}, CriteriaTotal: 4},
			wantEmpty: true,
		},
		{
			// The count is withheld below the anonymity gate, and absence must not be
			// printed as a number — "from 0 people" would be a claim the payload never made.
			name: "outcome evidence below the anonymity gate",
			in: &ghostSignal{Level: "possible", CriteriaTotal: 4,
				Criteria: []string{"evergreen_posting", "user_reports"}},
			wantSubstr: "People reported no response (reported)",
			wantAbsent: "0 people",
		},
		{
			name: "outcome evidence above the gate carries its count",
			in: &ghostSignal{Level: "likely", CriteriaTotal: 4, Contributors: &three,
				Criteria: []string{"silent_applications", "user_reports"}},
			wantSubstr: "from 3 people",
		},
		{
			// The scale's numerator must not disagree with the rows beneath it.
			name: "a criterion this build does not know",
			in: &ghostSignal{Level: "possible", CriteriaTotal: 5,
				Criteria: []string{"ats_absent", "employer_never_hired"}},
			wantSubstr: "employer_never_hired",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := strings.Join(c.in.lines(), "\n")
			if c.wantEmpty {
				if got != "" {
					t.Fatalf("lines = %q, want nothing", got)
				}
				return
			}
			if !strings.Contains(got, c.wantSubstr) {
				t.Errorf("lines = %q, want %q", got, c.wantSubstr)
			}
			if c.wantAbsent != "" && strings.Contains(got, c.wantAbsent) {
				t.Errorf("lines = %q, must not contain %q", got, c.wantAbsent)
			}
		})
	}
}
