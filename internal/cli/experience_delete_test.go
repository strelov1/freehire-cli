package cli

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// deleteRecorder is the experience fake plus a note of every DELETE that reached it, so a
// test can assert not only what the command printed but whether it actually removed
// anything — the two come apart exactly in the cases worth testing.
type deleteRecorder struct {
	mu   sync.Mutex
	seen []string
}

func (d *deleteRecorder) paths() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]string(nil), d.seen...)
}

// experienceDeleteAPI serves the same bank as experienceFakeAPI and records deletes.
func experienceDeleteAPI(t *testing.T) (*httptest.Server, *deleteRecorder) {
	t.Helper()
	rec := &deleteRecorder{}
	inner := experienceFakeAPI(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			rec.mu.Lock()
			rec.seen = append(rec.seen, r.URL.Path)
			rec.mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
			return
		}
		// Everything else is proxied to the shared fake by re-dialling it, which keeps one
		// definition of the bank rather than a second copy that can drift from it.
		req, err := http.NewRequestWithContext(r.Context(), r.Method, inner.URL+r.URL.RequestURI(), r.Body)
		if err != nil {
			t.Fatalf("proxy request: %v", err)
		}
		req.Header = r.Header.Clone()
		resp, err := inner.Client().Do(req)
		if err != nil {
			t.Fatalf("proxy: %v", err)
		}
		defer resp.Body.Close()
		w.WriteHeader(resp.StatusCode)
		buf := new(bytes.Buffer)
		_, _ = buf.ReadFrom(resp.Body)
		_, _ = w.Write(buf.Bytes())
	}))
	t.Cleanup(srv.Close)
	return srv, rec
}

// runIn is run() with something on stdin, for the commands that ask before destroying.
func runIn(t *testing.T, stdin string, args ...string) (string, error) {
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

const (
	emptyPlace    = "11111111-1111-4111-8111-111111111111"
	occupiedPlace = "33333333-3333-4333-8333-333333333333"
	unplacedAtom  = "22222222-2222-4222-8222-222222222222"
)

func TestExperienceAtomsRm(t *testing.T) {
	srv, rec := experienceDeleteAPI(t)
	experienceEnv(t, srv)
	out, err := run(t, "experience", "atoms", "rm", unplacedAtom, "--yes")
	if err != nil {
		t.Fatalf("atoms rm: %v", err)
	}
	if got := rec.paths(); len(got) != 1 || !strings.HasSuffix(got[0], unplacedAtom) {
		t.Errorf("deletes reaching the API = %v, want one for the atom", got)
	}
	if !strings.Contains(out, "Cut latency 20s to 1s") {
		t.Errorf("output %q should name what it removed", out)
	}
}

// The point of the whole feature: an emptied shell can go.
func TestExperienceEmploymentsRmEmptyPlace(t *testing.T) {
	srv, rec := experienceDeleteAPI(t)
	experienceEnv(t, srv)
	if _, err := run(t, "experience", "employments", "rm", emptyPlace, "--yes"); err != nil {
		t.Fatalf("employments rm: %v", err)
	}
	if got := rec.paths(); len(got) != 1 {
		t.Errorf("deletes reaching the API = %v, want one", got)
	}
}

// A place that still holds achievements is refused BEFORE the request. The server refuses it
// too (409), but stopping here lets the message name the way out, and means a caller who
// mistyped an id never even asks to destroy someone's record.
func TestExperienceEmploymentsRmRefusesAnOccupiedPlace(t *testing.T) {
	srv, rec := experienceDeleteAPI(t)
	experienceEnv(t, srv)
	_, err := run(t, "experience", "employments", "rm", occupiedPlace, "--yes")
	if err == nil {
		t.Fatal("removing a place that still holds achievements should error")
	}
	for _, want := range []string{"1", "atoms update"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q should mention %q", err, want)
		}
	}
	if got := rec.paths(); len(got) != 0 {
		t.Errorf("a refused removal still sent %v", got)
	}
}

// Without --yes the command asks, and only the exact word goes through. Anything else — an
// empty line, a stray "y", a piped newline — leaves the bank alone, because there is no undo
// and "probably meant yes" is not good enough for that.
func TestExperienceRmAsksBeforeDestroying(t *testing.T) {
	for _, tc := range []struct {
		name    string
		stdin   string
		deletes int
	}{
		{"the exact word", "delete\n", 1},
		{"a bare y is not enough", "y\n", 0},
		{"an empty answer", "\n", 0},
		{"nothing on stdin at all", "", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv, rec := experienceDeleteAPI(t)
			experienceEnv(t, srv)
			out, err := runIn(t, tc.stdin, "experience", "atoms", "rm", unplacedAtom)
			if tc.deletes == 0 && err == nil && !strings.Contains(out, "Cancelled") {
				t.Errorf("a declined removal should say so; output %q err %v", out, err)
			}
			if got := rec.paths(); len(got) != tc.deletes {
				t.Errorf("deletes = %v, want %d", got, tc.deletes)
			}
		})
	}
}

func TestExperienceRmUnknownID(t *testing.T) {
	srv, rec := experienceDeleteAPI(t)
	experienceEnv(t, srv)
	if _, err := run(t, "experience", "atoms", "rm", "55555555-5555-4555-8555-555555555555", "--yes"); err == nil {
		t.Error("removing an achievement that is not in the bank should error")
	}
	if got := rec.paths(); len(got) != 0 {
		t.Errorf("an unknown id still sent %v", got)
	}
}
