package client

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// accountMCPPath is the signed-in MCP server. Job alerts live only there: it is the
// one surface that holds their whole rule (the channels an account can receive on,
// the match ledger), so the CLI calls its tools rather than re-deriving them from
// the website's cookie-only routes.
const accountMCPPath = "/api/v1/mcp/account"

// ToolError is a tool that ran and refused: the server's own explanation, meant to be
// read by whoever made the call ("nothing was changed: …").
type ToolError struct{ Message string }

func (e *ToolError) Error() string { return e.Message }

// CallTool calls one tool on the signed-in MCP server and returns its JSON result.
//
// The server is stateless, so a call is a single JSON-RPC POST with no initialize
// handshake. It answers as a one-event SSE stream, which is why Accept names both
// types — the transport refuses a POST that does not.
func (c *Client) CallTool(ctx context.Context, name string, args any) (json.RawMessage, error) {
	payload, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{"name": name, "arguments": args},
	})
	if err != nil {
		return nil, err
	}
	resp, err := c.exchange(ctx, http.MethodPost, accountMCPPath, payload, "application/json, text/event-stream")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &APIError{Status: resp.StatusCode, Message: strings.TrimSpace(string(b))}
	}
	if strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		if b, err = sseData(b); err != nil {
			return nil, err
		}
	}

	var msg struct {
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
		Result struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
			Structured json.RawMessage `json:"structuredContent"`
			IsError    bool            `json:"isError"`
		} `json:"result"`
	}
	if err := json.Unmarshal(b, &msg); err != nil {
		return nil, fmt.Errorf("decode %s response: %w", name, err)
	}
	if msg.Error != nil {
		return nil, fmt.Errorf("%s: %s", name, msg.Error.Message)
	}
	var text string
	for _, c := range msg.Result.Content {
		if c.Type == "text" {
			text += c.Text
		}
	}
	if msg.Result.IsError {
		return nil, &ToolError{Message: text}
	}
	if len(msg.Result.Structured) > 0 && string(msg.Result.Structured) != "null" {
		return msg.Result.Structured, nil
	}
	if !json.Valid([]byte(text)) {
		return nil, fmt.Errorf("%s: unexpected result %q", name, text)
	}
	return json.RawMessage(text), nil
}

// sseData returns the data of the first event in an SSE body — a stateless tool
// call answers with exactly one.
func sseData(b []byte) ([]byte, error) {
	var data []byte
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if line == "" && data != nil {
			return data, nil
		}
		if rest, ok := strings.CutPrefix(line, "data:"); ok {
			if data != nil {
				data = append(data, '\n')
			}
			data = append(data, strings.TrimPrefix(rest, " ")...)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if data == nil {
		return nil, errors.New("empty event stream")
	}
	return data, nil
}
