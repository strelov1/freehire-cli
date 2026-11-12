package mcpproxy

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type echoArgs struct {
	Text string `json:"text"`
}

// hostedServer stands in for freehire's canonical endpoint: a stateless Streamable HTTP
// MCP server, as /api/v1/mcp is. It records the headers each request arrived with.
func hostedServer(t *testing.T, seen *[]http.Header, mu *sync.Mutex) *httptest.Server {
	t.Helper()
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		s := mcp.NewServer(&mcp.Implementation{Name: "freehire", Version: "test"}, &mcp.ServerOptions{Instructions: "hosted"})
		mcp.AddTool(s, &mcp.Tool{Name: "echo"}, func(_ context.Context, _ *mcp.CallToolRequest, in echoArgs) (*mcp.CallToolResult, any, error) {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: in.Text}}}, nil, nil
		})
		return s
	}, &mcp.StreamableHTTPOptions{Stateless: true})
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		*seen = append(*seen, r.Header.Clone())
		mu.Unlock()
		handler.ServeHTTP(w, r)
	}))
}

// connectThrough starts the relay between a pair of pipes and connects a real MCP client
// to its stdio end, the way a local host (Claude Code, Claude Desktop) launches it.
func connectThrough(t *testing.T, r *Relay) *mcp.ClientSession {
	t.Helper()
	hostOut, relayIn := io.Pipe()
	relayOut, hostIn := io.Pipe()
	done := make(chan error, 1)
	go func() { done <- r.Run(context.Background(), hostOut, hostIn) }()
	transport := &mcp.IOTransport{Reader: relayOut, Writer: relayIn}
	session, err := mcp.NewClient(&mcp.Implementation{Name: "host", Version: "0"}, nil).Connect(context.Background(), transport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = session.Close()
		if err := <-done; err != nil {
			t.Errorf("relay returned %v after stdin closed, want nil", err)
		}
	})
	return session
}

func TestRelay_ForwardsASessionAndIdentifiesTheCLI(t *testing.T) {
	var (
		seen []http.Header
		mu   sync.Mutex
	)
	srv := hostedServer(t, &seen, &mu)
	defer srv.Close()

	session := connectThrough(t, &Relay{Endpoint: srv.URL, Token: "tok", UserAgent: "freehire-cli/1.4.0"})
	if got := session.InitializeResult().Instructions; got != "hosted" {
		t.Fatalf("instructions = %q, want the hosted server's", got)
	}
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "echo", Arguments: map[string]any{"text": "hi"}})
	if err != nil || res.IsError {
		t.Fatalf("call: %v %v", res, err)
	}
	if text := res.Content[0].(*mcp.TextContent).Text; text != "hi" {
		t.Fatalf("echo = %q", text)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(seen) < 3 {
		t.Fatalf("server saw %d requests, want initialize, initialized and the call", len(seen))
	}
	for i, h := range seen {
		if h.Get("User-Agent") != "freehire-cli/1.4.0" || h.Get("Authorization") != "Bearer tok" {
			t.Errorf("request %d: User-Agent %q, Authorization %q", i, h.Get("User-Agent"), h.Get("Authorization"))
		}
	}
	if v := seen[len(seen)-1].Get("MCP-Protocol-Version"); v == "" {
		t.Errorf("the call after initialize carried no MCP-Protocol-Version")
	}
}

func TestRelay_AnonymousSendsNoAuthorization(t *testing.T) {
	var (
		seen []http.Header
		mu   sync.Mutex
	)
	srv := hostedServer(t, &seen, &mu)
	defer srv.Close()
	connectThrough(t, &Relay{Endpoint: srv.URL, UserAgent: "freehire-cli/1.4.0"})
	mu.Lock()
	defer mu.Unlock()
	for i, h := range seen {
		if h.Get("Authorization") != "" {
			t.Errorf("request %d sent Authorization %q without a token", i, h.Get("Authorization"))
		}
	}
}

// A refused request must reach the host as a JSON-RPC error for that request id —
// otherwise the host waits for an answer that never comes.
func TestRelay_AnHTTPRefusalIsAnErrorForThatRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "sign in to freehire to use personal tools", http.StatusUnauthorized)
	}))
	defer srv.Close()
	hostOut, relayIn := io.Pipe()
	relayOut, hostIn := io.Pipe()
	go func() {
		_ = (&Relay{Endpoint: srv.URL, UserAgent: "freehire-cli/t"}).Run(context.Background(), hostOut, hostIn)
	}()
	go func() {
		_, _ = io.WriteString(relayIn, `{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"my_jobs"}}`+"\n")
	}()
	line := make([]byte, 4096)
	n, _ := relayOut.Read(line)
	got := string(line[:n])
	for _, want := range []string{`"id":7`, `"error"`, "401", "freehire auth login"} {
		if !strings.Contains(got, want) {
			t.Errorf("answer %s lacks %s", got, want)
		}
	}
	_ = relayIn.Close()
}

// The edge challenges VPN and hosting networks; a signed-in relay goes around it through
// /api/v1/me/api, as every other CLI request does.
func TestRelay_AChallengedSignedInRequestGoesThroughTheSignedInDoor(t *testing.T) {
	var (
		seen []http.Header
		mu   sync.Mutex
	)
	hosted := hostedServer(t, &seen, &mu)
	defer hosted.Close()
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		if r.URL.Path == "/api/v1/mcp" {
			w.Header().Set("Cf-Mitigated", "challenge")
			w.WriteHeader(http.StatusForbidden)
			return
		}
		r.URL.Path = "/"
		resp, err := http.DefaultClient.Do(mustProxy(t, r, hosted.URL))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		for k, v := range resp.Header {
			w.Header()[k] = v
		}
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
	}))
	defer srv.Close()
	session := connectThrough(t, &Relay{Endpoint: srv.URL + "/api/v1/mcp", Token: "tok", UserAgent: "freehire-cli/t"})
	if session.InitializeResult().Instructions != "hosted" {
		t.Fatal("initialize did not get through the door")
	}
	mu.Lock()
	defer mu.Unlock()
	if paths[0] != "/api/v1/mcp" || paths[len(paths)-1] != "/api/v1/me/api/mcp" {
		t.Fatalf("paths = %v", paths)
	}
}

func mustProxy(t *testing.T, r *http.Request, to string) *http.Request {
	t.Helper()
	out, err := http.NewRequestWithContext(r.Context(), r.Method, to, r.Body)
	if err != nil {
		t.Fatal(err)
	}
	out.Header = r.Header.Clone()
	return out
}

// relayOnce sends lines to a relay in front of handler and returns everything it wrote
// once stdin has closed and the requests in flight are answered.
func relayOnce(t *testing.T, handler http.HandlerFunc, lines ...string) (string, error) {
	t.Helper()
	srv := httptest.NewServer(handler)
	defer srv.Close()
	var out strings.Builder
	err := (&Relay{Endpoint: srv.URL, UserAgent: "freehire-cli/t"}).Run(context.Background(),
		strings.NewReader(strings.Join(lines, "\n")+"\n"), &out)
	return out.String(), err
}

// The relay reads a message only to route it; one it cannot read that way still goes
// to the server verbatim rather than ending the session.
func TestRelay_AMessageItCannotRouteIsStillForwarded(t *testing.T) {
	var got string
	out, err := relayOnce(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got = string(b)
		w.WriteHeader(http.StatusAccepted)
	}, `{"jsonrpc":"2.0","method":"notifications/x","params":["positional"]}`)
	if err != nil || out != "" {
		t.Fatalf("Run = %v, wrote %q", err, out)
	}
	if got != `{"jsonrpc":"2.0","method":"notifications/x","params":["positional"]}` {
		t.Fatalf("server got %q", got)
	}
}

// Only a request is owed an answer: a refused response from the host is not answered
// with an error the host would read as a reply to a request it never sent.
func TestRelay_ARefusedResponseIsNotAnsweredWithAnError(t *testing.T) {
	out, err := relayOnce(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}, `{"jsonrpc":"2.0","id":5,"result":{}}`)
	if err != nil || out != "" {
		t.Fatalf("Run = %v, wrote %q", err, out)
	}
}

// A request answered 2xx with nothing the relay can read still gets an answer.
func TestRelay_ASuccessWithNoReadableBodyIsAnErrorForTheRequest(t *testing.T) {
	out, err := relayOnce(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, "hello")
	}, `{"jsonrpc":"2.0","id":4,"method":"tools/list"}`)
	if err != nil || !strings.Contains(out, `"id":4`) || !strings.Contains(out, `"error"`) {
		t.Fatalf("Run = %v, wrote %q", err, out)
	}
}

// A stream that ends after a progress notification but before the response leaves the
// request owed an answer.
func TestRelay_AStreamThatEndsWithoutTheResponseIsAnError(t *testing.T) {
	out, err := relayOnce(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "event: message\ndata: {\"jsonrpc\":\"2.0\",\"method\":\"notifications/progress\",\"params\":{}}\n\n")
	}, `{"jsonrpc":"2.0","id":"a","method":"tools/call","params":{}}`)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if err != nil || len(lines) != 2 || !strings.Contains(lines[0], "notifications/progress") ||
		!strings.Contains(lines[1], `"id":"a"`) || !strings.Contains(lines[1], `"error"`) {
		t.Fatalf("Run = %v, wrote %q", err, out)
	}
}

// Notifications go out in order before the next message: notifications/initialized must
// reach the server before the request that follows it.
func TestRelay_ANotificationIsSentBeforeTheNextMessage(t *testing.T) {
	var (
		mu    sync.Mutex
		order []string
	)
	_, err := relayOnce(t, func(w http.ResponseWriter, r *http.Request) {
		var m struct {
			Method string `json:"method"`
		}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &m)
		if m.Method == "notifications/initialized" {
			time.Sleep(50 * time.Millisecond) // a slow notification must still come first
		}
		mu.Lock()
		order = append(order, m.Method)
		mu.Unlock()
		if m.Method == "tools/list" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"jsonrpc":"2.0","id":2,"result":{}}`)
			return
		}
		w.WriteHeader(http.StatusAccepted)
	}, `{"jsonrpc":"2.0","method":"notifications/initialized"}`, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	if err != nil || len(order) != 2 || order[0] != "notifications/initialized" {
		t.Fatalf("Run = %v, order %v", err, order)
	}
}

// Once the host closes stdin it is going away: a request that never ends is cancelled
// after the grace period instead of keeping the process alive.
func TestRelay_ClosedStdinBoundsTheWaitForRequestsInFlight(t *testing.T) {
	defer func(g time.Duration) { shutdownGrace = g }(shutdownGrace)
	shutdownGrace = 100 * time.Millisecond
	start := time.Now()
	_, err := relayOnce(t, func(_ http.ResponseWriter, r *http.Request) {
		// The server notices the client went away only once the body is read.
		_, _ = io.ReadAll(r.Body)
		<-r.Context().Done()
	}, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{}}`)
	if err != nil || time.Since(start) > 5*time.Second {
		t.Fatalf("Run = %v after %v", err, time.Since(start))
	}
}

func TestRelay_AnUnauthorizedAnswerSaysToRestartAfterSigningIn(t *testing.T) {
	out, _ := relayOnce(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{}}`)
	if !strings.Contains(out, "freehire auth login") || !strings.Contains(out, "restart") {
		t.Fatalf("wrote %q", out)
	}
}

// A message sent in order (initialize, a notification) that stalls must not keep the
// relay from seeing stdin close: the grace period still starts and still ends it.
func TestRelay_ClosedStdinBoundsAStalledInOrderMessage(t *testing.T) {
	defer func(g time.Duration) { shutdownGrace = g }(shutdownGrace)
	shutdownGrace = 100 * time.Millisecond
	start := time.Now()
	_, err := relayOnce(t, func(_ http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		<-r.Context().Done()
	}, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	if err != nil || time.Since(start) > 5*time.Second {
		t.Fatalf("Run = %v after %v", err, time.Since(start))
	}
}
