// Package mcpproxy relays an MCP session from stdio to freehire's hosted Streamable
// HTTP endpoint, so a local host (Claude Code, Claude Desktop) reaches the same server,
// tools and instructions as a remote connector, signed in with the CLI's credentials.
//
// It forwards messages verbatim rather than terminating the protocol in an SDK client:
// the hosted server is the one implementation, and a relay that understood the
// messages would be a second place for them to drift apart.
package mcpproxy

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
	"sync"
	"time"
)

// signedInDoor is the API's second address for /api/v1, open only to a caller holding
// a credential. The edge challenges VPN and hosting networks — a challenge no CLI can
// solve — and lets /api/v1/me/ through. The CLI's API client uses the same door.
const signedInDoor = "/api/v1/me/api"

// shutdownGrace is how long requests in flight may still finish once stdin closes.
var shutdownGrace = 10 * time.Second

const metaKeyProtocolVersion = "io.modelcontextprotocol/protocolVersion"

// Relay copies newline-delimited JSON-RPC messages from stdin to Endpoint, one POST per
// message, and writes every message the server answers with back as one line.
type Relay struct {
	Endpoint  string // e.g. https://freehire.me/api/v1/mcp
	Token     string // empty: an anonymous session, public tools only
	UserAgent string
	HTTP      *http.Client // nil: http.DefaultClient

	mu              sync.Mutex
	sessionID       string
	protocolVersion string
	throughDoor     bool

	outMu sync.Mutex
	out   io.Writer
}

// message is the part of a JSON-RPC message the relay reads to route it.
type message struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params struct {
		Meta map[string]any `json:"_meta,omitempty"`
	} `json:"params"`
	Result struct {
		ProtocolVersion string `json:"protocolVersion"`
	} `json:"result"`
}

// Run relays until in ends, then lets the messages in flight finish for up to
// shutdownGrace before cancelling them, and returns nil.
//
// A request is sent concurrently: a host may issue a call while another is pending, and
// a long tool call must not hold up a cancellation behind it. Everything else — the
// initialize request, notifications, the host's responses — is sent in order: nothing
// after it is sent until it has been, since notifications/initialized must reach the
// server before the request that follows it and every later request carries the version
// initialize settles. The order is kept by a chain of gates rather than by sending inline,
// so the read loop never blocks on the server and always sees stdin close.
func (r *Relay) Run(ctx context.Context, in io.Reader, out io.Writer) error {
	r.out = out
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var wg sync.WaitGroup
	defer func() {
		finished := make(chan struct{})
		go func() { wg.Wait(); close(finished) }()
		select {
		case <-finished:
		case <-time.After(shutdownGrace):
			cancel()
			<-finished
		}
	}()

	// gate closes once the last in-order message has been sent.
	gate := make(chan struct{})
	close(gate)
	reader := bufio.NewReader(in)
	for {
		line, err := reader.ReadBytes('\n')
		if line = bytes.TrimSpace(line); len(line) > 0 {
			// The fields are read only to route the message; one that does not fit them
			// (positional params, say) is still forwarded verbatim, unrouted.
			var msg message
			_ = json.Unmarshal(line, &msg)
			after := gate
			var sent chan struct{}
			if !msg.isRequest() || msg.Method == "initialize" {
				sent = make(chan struct{})
				gate = sent
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				if sent != nil {
					defer close(sent)
				}
				select {
				case <-after:
					r.forward(ctx, line, msg)
				case <-ctx.Done():
					r.fail(msg, "the MCP host closed the connection")
				}
			}()
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

// isRequest reports a message that expects an answer: a method with an id.
func (m message) isRequest() bool { return m.Method != "" && len(m.ID) > 0 }

// forward sends one message and relays the answer. A failure answers a request with a
// JSON-RPC error for its id — the host otherwise waits for a reply that never comes.
func (r *Relay) forward(ctx context.Context, body []byte, msg message) {
	resp, err := r.post(ctx, body, msg)
	if err != nil {
		r.fail(msg, err.Error())
		return
	}
	defer resp.Body.Close()
	if id := resp.Header.Get("Mcp-Session-Id"); id != "" {
		r.mu.Lock()
		r.sessionID = id
		r.mu.Unlock()
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		text, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		reason := fmt.Sprintf("freehire MCP answered HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(text)))
		if resp.StatusCode == http.StatusUnauthorized {
			reason += " (run `freehire auth login`, then restart this MCP server)"
		}
		r.fail(msg, reason)
		return
	}
	mediaType, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	switch mediaType {
	case "text/event-stream":
		r.relayEvents(resp.Body, msg)
	case "application/json":
		payload, err := io.ReadAll(resp.Body)
		if err != nil {
			r.fail(msg, "reading the freehire MCP answer: "+err.Error())
			return
		}
		// Checked before relaying: a stray answer could be taken by the host for the
		// response to another request it has in flight.
		if msg.isRequest() && !answers(payload, msg) {
			r.fail(msg, "the freehire MCP answer is not the response to this request")
			return
		}
		r.emit(payload, msg)
	default:
		// 202 Accepted answers a notification or a response and carries nothing to relay;
		// anything else, and a request accepted that way, leaves a request unanswered
		// unless it is told so.
		if resp.StatusCode != http.StatusAccepted || msg.isRequest() {
			r.fail(msg, fmt.Sprintf("freehire MCP answered HTTP %d with unreadable content (%q)",
				resp.StatusCode, resp.Header.Get("Content-Type")))
		}
	}
}

func (r *Relay) post(ctx context.Context, body []byte, msg message) (*http.Response, error) {
	resp, err := r.send(ctx, body, msg)
	if err != nil || !edgeChallenged(resp) {
		return resp, err
	}
	resp.Body.Close()
	if r.Token == "" {
		return nil, errors.New("blocked by the network edge, which challenges VPN and hosting networks: " +
			"retry off the VPN, or sign in with `freehire auth login` (signed-in requests go around it)")
	}
	r.mu.Lock()
	r.throughDoor = true
	r.mu.Unlock()
	return r.send(ctx, body, msg)
}

func (r *Relay) send(ctx context.Context, body []byte, msg message) (*http.Response, error) {
	r.mu.Lock()
	endpoint, sessionID, version := r.Endpoint, r.sessionID, r.protocolVersion
	if r.throughDoor {
		endpoint = doorURL(endpoint)
	}
	r.mu.Unlock()
	if v, ok := msg.Params.Meta[metaKeyProtocolVersion].(string); ok && v != "" {
		version = v
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("User-Agent", r.UserAgent)
	if r.Token != "" {
		req.Header.Set("Authorization", "Bearer "+r.Token)
	}
	if sessionID != "" {
		req.Header.Set("Mcp-Session-Id", sessionID)
	}
	if version != "" {
		req.Header.Set("MCP-Protocol-Version", version)
	}
	client := r.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	return client.Do(req)
}

// relayEvents writes the data of every server-sent event as one message.
func (r *Relay) relayEvents(body io.Reader, sent message) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	var data []string
	answered := false
	flush := func() {
		if len(data) > 0 {
			event := []byte(strings.Join(data, "\n"))
			answered = answered || answers(event, sent)
			r.emit(event, sent)
			data = nil
		}
	}
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case line == "":
			flush()
		case strings.HasPrefix(line, "data:"):
			data = append(data, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		}
	}
	flush()
	switch {
	case scanner.Err() != nil:
		r.fail(sent, "reading the freehire MCP stream: "+scanner.Err().Error())
	case !answered:
		r.fail(sent, "the freehire MCP stream ended without a response")
	}
}

// answers reports whether event is the response to the request sent.
func answers(event []byte, sent message) bool {
	var m struct {
		ID     json.RawMessage `json:"id"`
		Method string          `json:"method"`
	}
	if json.Unmarshal(event, &m) != nil || m.Method != "" || len(m.ID) == 0 {
		return false
	}
	var a, b bytes.Buffer
	return json.Compact(&a, m.ID) == nil && json.Compact(&b, sent.ID) == nil && a.String() == b.String()
}

// emit writes one server message as a single line, and learns the negotiated protocol
// version from the answer to initialize.
func (r *Relay) emit(payload []byte, sent message) {
	// An SSE priming event (an id with empty data) carries no message.
	if len(bytes.TrimSpace(payload)) == 0 {
		return
	}
	var line bytes.Buffer
	if err := json.Compact(&line, payload); err != nil {
		r.fail(sent, "the freehire MCP answer is not JSON")
		return
	}
	if sent.Method == "initialize" {
		var answer message
		if json.Unmarshal(line.Bytes(), &answer) == nil && answer.Result.ProtocolVersion != "" {
			r.mu.Lock()
			r.protocolVersion = answer.Result.ProtocolVersion
			r.mu.Unlock()
		}
	}
	line.WriteByte('\n')
	r.outMu.Lock()
	defer r.outMu.Unlock()
	_, _ = r.out.Write(line.Bytes())
}

// fail answers a request with a JSON-RPC error. A notification, or the host's own
// response to a server request, is owed no answer.
func (r *Relay) fail(sent message, reason string) {
	if !sent.isRequest() {
		return
	}
	payload, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      sent.ID,
		"error":   map[string]any{"code": -32603, "message": reason},
	})
	r.emit(payload, message{})
}

// edgeChallenged reports a Cloudflare challenge: a 403 the API never produced.
func edgeChallenged(resp *http.Response) bool {
	return resp.StatusCode == http.StatusForbidden && resp.Header.Get("Cf-Mitigated") == "challenge"
}

// doorURL is endpoint's /api/v1 path moved under the signed-in door.
func doorURL(endpoint string) string {
	i := strings.Index(endpoint, "/api/v1/")
	if i < 0 {
		return endpoint
	}
	return endpoint[:i] + signedInDoor + endpoint[i+len("/api/v1"):]
}
