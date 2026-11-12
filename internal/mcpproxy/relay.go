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
)

// signedInDoor is the API's second address for /api/v1, open only to a caller holding
// a credential. The edge challenges VPN and hosting networks — a challenge no CLI can
// solve — and lets /api/v1/me/ through. The CLI's API client uses the same door.
const signedInDoor = "/api/v1/me/api"

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

// Run relays until in ends, then waits for the requests in flight and returns nil.
// Each message is sent concurrently: a host may issue a call while another is pending,
// and a long tool call must not hold up a cancellation behind it.
func (r *Relay) Run(ctx context.Context, in io.Reader, out io.Writer) error {
	r.out = out
	reader := bufio.NewReader(in)
	var wg sync.WaitGroup
	defer wg.Wait()
	for {
		line, err := reader.ReadBytes('\n')
		if line = bytes.TrimSpace(line); len(line) > 0 {
			// The fields are read only to route the message; one that does not fit them
			// (positional params, say) is still forwarded verbatim, unrouted.
			var msg message
			_ = json.Unmarshal(line, &msg)
			if msg.Method == "initialize" {
				// Every later request carries the version initialize settles, so it
				// must be answered before anything else is sent.
				r.forward(ctx, line, msg)
			} else {
				wg.Add(1)
				go func() {
					defer wg.Done()
					r.forward(ctx, line, msg)
				}()
			}
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

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
			reason += " (run `freehire auth login`)"
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
		r.emit(payload, msg)
	}
	// 202 Accepted (a notification or a response) carries nothing to relay.
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
	flush := func() {
		if len(data) > 0 {
			r.emit([]byte(strings.Join(data, "\n")), sent)
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
	if err := scanner.Err(); err != nil {
		r.fail(sent, "reading the freehire MCP stream: "+err.Error())
	}
}

// emit writes one server message as a single line, and learns the negotiated protocol
// version from the answer to initialize.
func (r *Relay) emit(payload []byte, sent message) {
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
	if len(sent.ID) == 0 || sent.Method == "" {
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
