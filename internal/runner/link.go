package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/coder/websocket"
)

// DialOptions describes the device's connection to the server.
type DialOptions struct {
	// ServerURL is the management service's base, e.g.
	// https://agent.freehire.dev. A ws:// or wss:// URL is accepted too, since
	// that is what users paste.
	ServerURL string
	// Token is the session JWT. Carried in the WebSocket subprotocol slot
	// because browsers cannot set headers on an upgrade and the server accepts
	// the same carrier from every client.
	Token string
	// DeviceID identifies this machine. The server namespaces it by the
	// authenticated user, so what we send is the bare id.
	DeviceID string
	// Harnesses this device can start. Must match what LookupHarness accepts,
	// or the server would route a spawn here that we then refuse.
	Harnesses []string
}

// wsLink is the tunnel over a WebSocket. It satisfies `link`.
type wsLink struct {
	conn *websocket.Conn
}

// dial connects, then registers. Registration is sent here rather than by the
// caller because the server's connection is a command stream until it arrives —
// any other first message would be parsed as a command and rejected.
func dial(ctx context.Context, opt DialOptions) (*wsLink, error) {
	if strings.TrimSpace(opt.Token) == "" {
		return nil, errors.New("no token: set ROY_RUNNER_TOKEN or pass --token")
	}
	if strings.TrimSpace(opt.DeviceID) == "" {
		return nil, errors.New("no device id")
	}
	endpoint, err := runnerEndpoint(opt)
	if err != nil {
		return nil, err
	}

	conn, _, err := websocket.Dial(ctx, endpoint, &websocket.DialOptions{
		Subprotocols: []string{"roy-jwt", opt.Token},
	})
	if err != nil {
		return nil, fmt.Errorf("connecting to %s: %w", opt.ServerURL, err)
	}
	// An agent protocol line can be large; the default read limit would close
	// the connection mid-conversation.
	conn.SetReadLimit(16 * 1024 * 1024)

	l := &wsLink{conn: conn}
	if err := l.send(ctx, Register(opt.DeviceID, opt.Harnesses)); err != nil {
		_ = conn.CloseNow()
		return nil, fmt.Errorf("registering device: %w", err)
	}
	return l, nil
}

// runnerEndpoint builds the upgrade URL, carrying identity in the query string.
// It lives there rather than in a first tunnel message so roy-management can
// bridge the connection without parsing tunnel traffic.
func runnerEndpoint(opt DialOptions) (string, error) {
	u, err := url.Parse(strings.TrimRight(opt.ServerURL, "/"))
	if err != nil {
		return "", fmt.Errorf("bad server url %q: %w", opt.ServerURL, err)
	}
	switch u.Scheme {
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	case "ws", "wss":
		// already a websocket URL
	default:
		return "", fmt.Errorf("unsupported scheme %q in server url", u.Scheme)
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/runners/ws"

	q := u.Query()
	q.Set("device_id", opt.DeviceID)
	q.Set("version", fmt.Sprint(TunnelVersion))
	if len(opt.Harnesses) > 0 {
		q.Set("harnesses", strings.Join(opt.Harnesses, ","))
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func (l *wsLink) send(ctx context.Context, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return l.conn.Write(ctx, websocket.MessageText, b)
}

// Send delivers one tunnel message.
func (l *wsLink) Send(ctx context.Context, m Message) error { return l.send(ctx, m) }

// Recv returns the next tunnel message. An undecodable frame is skipped rather
// than fatal: a newer server may send something this version does not model,
// and dropping the connection over it would be worse than ignoring it.
func (l *wsLink) Recv(ctx context.Context) (Message, error) {
	for {
		_, data, err := l.conn.Read(ctx)
		if err != nil {
			return Message{}, err
		}
		var m Message
		if err := json.Unmarshal(data, &m); err != nil {
			continue
		}
		return m, nil
	}
}

func (l *wsLink) Close() error { return l.conn.CloseNow() }
