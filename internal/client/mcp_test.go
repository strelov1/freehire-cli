package client

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// sseServer answers every tools/call with one SSE event carrying result, and records
// the request it got.
func sseServer(t *testing.T, result string, got *map[string]any) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/mcp/account" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if a := r.Header.Get("Accept"); !strings.Contains(a, "text/event-stream") || !strings.Contains(a, "application/json") {
			t.Errorf("Accept = %q, want both types (the transport refuses a POST without them)", a)
		}
		b, _ := io.ReadAll(r.Body)
		if got != nil {
			json.Unmarshal(b, got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("event: message\ndata: {\"jsonrpc\":\"2.0\",\"id\":1,\"result\":" + result + "}\n\n"))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestCallTool_ReadsTextResultFromEventStream(t *testing.T) {
	var req map[string]any
	srv := sseServer(t, `{"content":[{"type":"text","text":"{\"alerts\":[]}"}]}`, &req)
	data, err := New(srv.URL, "tok", nil).CallTool(context.Background(), "list_subscriptions", map[string]any{"id": 3})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if string(data) != `{"alerts":[]}` {
		t.Errorf("data = %s", data)
	}
	params, _ := req["params"].(map[string]any)
	if req["method"] != "tools/call" || params["name"] != "list_subscriptions" {
		t.Errorf("request = %v, want a tools/call of list_subscriptions", req)
	}
}

func TestCallTool_PrefersStructuredContent(t *testing.T) {
	srv := sseServer(t, `{"content":[{"type":"text","text":"summary"}],"structuredContent":{"ok":true}}`, nil)
	data, err := New(srv.URL, "tok", nil).CallTool(context.Background(), "x", nil)
	if err != nil || string(data) != `{"ok":true}` {
		t.Errorf("data = %s, err = %v", data, err)
	}
}

func TestCallTool_ToolRefusalIsAToolError(t *testing.T) {
	srv := sseServer(t, `{"content":[{"type":"text","text":"nothing was changed: no such alert"}],"isError":true}`, nil)
	_, err := New(srv.URL, "tok", nil).CallTool(context.Background(), "x", nil)
	var te *ToolError
	if !errors.As(err, &te) || te.Message != "nothing was changed: no such alert" {
		t.Errorf("err = %v, want the server's sentence as a ToolError", err)
	}
}

func TestCallTool_Non2xxIsAnAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)
	_, err := New(srv.URL, "bad", nil).CallTool(context.Background(), "x", nil)
	var ae *APIError
	if !errors.As(err, &ae) || ae.Status != http.StatusUnauthorized {
		t.Errorf("err = %v, want a 401 APIError", err)
	}
}
