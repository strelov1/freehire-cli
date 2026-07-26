package runner

import (
	"encoding/json"
	"strings"
	"testing"
)

// Wire samples exactly as the server emits them. These are the contract between
// two repositories: if the Rust side changes a tag or a field name, these fail
// here rather than at runtime on a user's laptop.
func TestServerMessagesDecode(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want func(*testing.T, Message)
	}{
		{
			name: "open",
			raw:  `{"t":"open","stream_id":7,"harness":"claude","model":"claude-haiku-4-5-20251001","env":{"FREEHIRE_TOKEN":"sk-x","ROY_SESSION_ID":"sess-1"}}`,
			want: func(t *testing.T, m Message) {
				if m.Type != MsgOpen {
					t.Fatalf("type = %q", m.Type)
				}
				if m.StreamID != 7 || m.Harness != "claude" {
					t.Fatalf("unexpected open: %+v", m)
				}
				if m.Env["ROY_SESSION_ID"] != "sess-1" {
					t.Fatalf("env not decoded: %+v", m.Env)
				}
			},
		},
		{
			name: "frame",
			raw:  `{"t":"frame","stream_id":7,"data":"{\"jsonrpc\":\"2.0\"}"}`,
			want: func(t *testing.T, m Message) {
				if m.Type != MsgFrame || m.Data != `{"jsonrpc":"2.0"}` {
					t.Fatalf("unexpected frame: %+v", m)
				}
			},
		},
		{
			name: "close",
			raw:  `{"t":"close","stream_id":7}`,
			want: func(t *testing.T, m Message) {
				if m.Type != MsgClose || m.StreamID != 7 {
					t.Fatalf("unexpected close: %+v", m)
				}
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var m Message
			if err := json.Unmarshal([]byte(c.raw), &m); err != nil {
				t.Fatalf("decode: %v", err)
			}
			c.want(t, m)
		})
	}
}

func TestRunnerMessagesEncodeToTheServersShape(t *testing.T) {
	cases := []struct {
		name  string
		msg   Message
		wants []string
	}{
		{"opened", Opened(7), []string{`"t":"opened"`, `"stream_id":7`}},
		{"open_failed", OpenFailed(7, "claude-code-acp not found on PATH"), []string{
			`"t":"open_failed"`, `"reason":"claude-code-acp not found on PATH"`,
		}},
		{"frame", Frame(7, "line"), []string{`"t":"frame"`, `"data":"line"`}},
		{"closed", Closed(7, 1), []string{`"t":"closed"`, `"code":1`}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b, err := json.Marshal(c.msg)
			if err != nil {
				t.Fatalf("encode: %v", err)
			}
			for _, want := range c.wants {
				if !strings.Contains(string(b), want) {
					t.Errorf("encoded %s missing %s: %s", c.name, want, b)
				}
			}
		})
	}
}

func TestEmptyFieldsAreOmitted(t *testing.T) {
	// The server's enum has no room for fields a variant does not carry, so a
	// stray key would fail to decode there.
	b, _ := json.Marshal(Opened(3))
	for _, absent := range []string{"harness", "data", "reason", "env", "code"} {
		if strings.Contains(string(b), absent) {
			t.Errorf("opened must not carry %q: %s", absent, b)
		}
	}
}

func TestRegistrationMatchesTheServersCommand(t *testing.T) {
	// Registration is a ClientCommand, not a tunnel message: it is what turns
	// the connection into a tunnel in the first place.
	b, err := json.Marshal(Register("dev-1", []string{"claude"}))
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	for _, want := range []string{
		`"op":"register_runner"`,
		`"runner_id":"dev-1"`,
		`"version":1`,
		`"harnesses":["claude"]`,
	} {
		if !strings.Contains(string(b), want) {
			t.Errorf("registration missing %s: %s", want, b)
		}
	}
}
