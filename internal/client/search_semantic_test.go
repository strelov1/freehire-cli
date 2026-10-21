package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func semanticProbe(t *testing.T, semantic string) (*httptest.Server, *url.Values) {
	t.Helper()
	var got url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Query()
		w.Write([]byte(`{"data":[],"meta":{"total":0,"semantic":"` + semantic + `"}}`))
	}))
	t.Cleanup(srv.Close)
	return srv, &got
}

func TestSearch_SemanticIsSentOnlyWhenAsked(t *testing.T) {
	srv, got := semanticProbe(t, "")
	c := New(srv.URL, "", srv.Client())

	if _, err := c.Search(context.Background(), SearchParams{Query: "go", Limit: 5}); err != nil {
		t.Fatalf("Search: %v", err)
	}
	if got.Has("semantic") {
		t.Errorf("plain search sent semantic=%q, want the param absent", got.Get("semantic"))
	}

	if _, err := c.Search(context.Background(), SearchParams{Query: "go", Limit: 5, Semantic: true}); err != nil {
		t.Fatalf("Search: %v", err)
	}
	if got.Get("semantic") != "true" {
		t.Errorf("semantic search sent semantic=%q, want true", got.Get("semantic"))
	}
}

func TestSearch_SurfacesWhatBecameOfSemantic(t *testing.T) {
	// "unavailable" means the answer is keyword-only. The caller must be able to tell,
	// or a fallback reads as a meaning-based result.
	srv, _ := semanticProbe(t, "unavailable")
	c := New(srv.URL, "", srv.Client())

	page, err := c.Search(context.Background(), SearchParams{Query: "go", Limit: 5, Semantic: true})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if page.Semantic != "unavailable" {
		t.Errorf("Semantic = %q, want unavailable", page.Semantic)
	}
}
