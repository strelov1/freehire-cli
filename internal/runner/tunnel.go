package runner

// The tunnel wire format, mirroring `roy_protocol::tunnel` on the server. One
// JSON object per WebSocket text frame; every message names the stream it
// belongs to, because a single connection carries every session on this device.
//
// Kept as one struct rather than a type per variant: the set is small, the
// fields barely overlap, and a flat shape decodes without a two-pass sniff of
// the tag. Fields absent from a variant are omitted, since the server's enum
// has nowhere to put a stray key.

// TunnelVersion is the protocol this runner speaks. The server refuses a
// mismatch at registration rather than later on an unknown message, so a stale
// binary fails with something a user can act on.
const TunnelVersion = 1

// Message types, matching the server's `#[serde(tag = "t")]` names.
const (
	MsgOpen       = "open"
	MsgOpened     = "opened"
	MsgOpenFailed = "open_failed"
	MsgFrame      = "frame"
	MsgClose      = "close"
	MsgClosed     = "closed"
)

// Message is one tunnel message in either direction.
type Message struct {
	Type     string `json:"t"`
	StreamID uint64 `json:"stream_id"`

	// Open only. Harness is an identifier from a set this runner knows; it is
	// never a command. Env arrives pre-filtered by the server, and is applied
	// on top of the current environment rather than replacing it.
	Harness string            `json:"harness,omitempty"`
	Model   string            `json:"model,omitempty"`
	Env     map[string]string `json:"env,omitempty"`

	// Frame only: one line of the agent protocol, verbatim.
	Data string `json:"data,omitempty"`

	// OpenFailed only: why the harness did not start.
	Reason string `json:"reason,omitempty"`

	// Closed only: the harness's exit code.
	Code *int `json:"code,omitempty"`
}

// Opened reports that the harness started and the stream is live.
func Opened(stream uint64) Message {
	return Message{Type: MsgOpened, StreamID: stream}
}

// OpenFailed reports that the harness did not start, and why. The reason
// reaches the user, so it should name the problem ("not found on PATH"), not
// just its category.
func OpenFailed(stream uint64, reason string) Message {
	return Message{Type: MsgOpenFailed, StreamID: stream, Reason: reason}
}

// Frame carries one line of the agent protocol.
func Frame(stream uint64, data string) Message {
	return Message{Type: MsgFrame, StreamID: stream, Data: data}
}

// Closed reports that the harness exited.
func Closed(stream uint64, code int) Message {
	return Message{Type: MsgClosed, StreamID: stream, Code: &code}
}

// Registration is `ClientCommand::RegisterRunner`: not a tunnel message, but
// the command that turns the connection into a tunnel. The server namespaces
// RunnerID by the authenticated user, so the value here is this device's id
// alone.
type Registration struct {
	Op        string   `json:"op"`
	RunnerID  string   `json:"runner_id"`
	Version   int      `json:"version"`
	Harnesses []string `json:"harnesses"`
}

// Register builds the registration command for this device.
func Register(deviceID string, harnesses []string) Registration {
	return Registration{
		Op:        "register_runner",
		RunnerID:  deviceID,
		Version:   TunnelVersion,
		Harnesses: harnesses,
	}
}
