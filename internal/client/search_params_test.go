package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// searchProbe stands in for the API and captures the query the client sent,
// answering with a response that carries an ignored-param warning.
func searchProbe(t *testing.T) (*httptest.Server, *url.Values) {
	t.Helper()
	var got url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Query()
		w.Write([]byte(`{"data":[],"meta":{"total":0,"ignored_params":[` +
			`{"param":"country","did_you_mean":"countries"},{"param":"utm_source"}]}}`))
	}))
	t.Cleanup(srv.Close)
	return srv, &got
}

func TestSearch_SurfacesIgnoredParams(t *testing.T) {
	// The API drops a param no filter reads. The CLI has to carry that warning
	// out of the envelope, or the user reads an unfiltered count as an answer.
	srv, _ := searchProbe(t)
	c := New(srv.URL, "", srv.Client())

	page, err := c.Search(context.Background(), SearchParams{Query: "go", Limit: 5})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	if len(page.Ignored) != 2 {
		t.Fatalf("Ignored = %#v, want two entries", page.Ignored)
	}
	if page.Ignored[0].Param != "country" || page.Ignored[0].DidYouMean != "countries" {
		t.Errorf("Ignored[0] = %#v, want country -> countries", page.Ignored[0])
	}
	if page.Ignored[1].DidYouMean != "" {
		t.Errorf("Ignored[1].DidYouMean = %q, want empty", page.Ignored[1].DidYouMean)
	}
}

func TestSearch_SendsNoParamTheAPIDoesNotRead(t *testing.T) {
	// semantic_ratio died with the hybrid index and include_description was never
	// read; both would now come back as warnings the user cannot act on.
	srv, got := searchProbe(t)
	c := New(srv.URL, "", srv.Client())

	if _, err := c.Search(context.Background(), SearchParams{Query: "go", Limit: 5}); err != nil {
		t.Fatalf("Search: %v", err)
	}

	for _, dead := range []string{"semantic_ratio", "include_description"} {
		if got.Has(dead) {
			t.Errorf("query still carries %s=%q", dead, got.Get(dead))
		}
	}
}

func TestFacetsAndCoverage_SurfaceIgnoredParams(t *testing.T) {
	// These two turn a filter into a number — a vacancy count, a coverage
	// percentage. A dropped filter does not make the answer look long, it makes
	// it look wrong-but-confident, so the warning has to reach the caller here
	// as much as it does on search.
	srv, _ := searchProbe(t)
	c := New(srv.URL, "", srv.Client())

	facets, err := c.Facets(context.Background(), url.Values{"country": {"it"}})
	if err != nil {
		t.Fatalf("Facets: %v", err)
	}
	if len(facets.Ignored) != 2 || facets.Ignored[0].DidYouMean != "countries" {
		t.Errorf("Facets ignored = %#v, want the country warning", facets.Ignored)
	}

	coverage, err := c.Coverage(context.Background(), CoverageParams{Skills: []string{"go"}})
	if err != nil {
		t.Fatalf("Coverage: %v", err)
	}
	if len(coverage.Ignored) != 2 {
		t.Errorf("Coverage ignored = %#v, want the warnings carried through", coverage.Ignored)
	}
}
