package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// wsServer accepts one runner connection and hands the test what it received.
type wsServer struct {
	*httptest.Server
	gotPath  chan string
	gotProto chan string
	gotFirst chan string
	toRunner chan Message
}

func newWSServer(t *testing.T) *wsServer {
	t.Helper()
	s := &wsServer{
		gotPath:  make(chan string, 1),
		gotProto: make(chan string, 1),
		gotFirst: make(chan string, 1),
		toRunner: make(chan Message, 4),
	}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.gotPath <- r.URL.String()
		s.gotProto <- r.Header.Get("Sec-WebSocket-Protocol")
		c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			Subprotocols: []string{"roy-jwt"},
		})
		if err != nil {
			return
		}
		defer c.CloseNow()
		ctx := r.Context()

		_, first, err := c.Read(ctx)
		if err != nil {
			return
		}
		s.gotFirst <- string(first)

		for {
			select {
			case m := <-s.toRunner:
				b, _ := json.Marshal(m)
				if err := c.Write(ctx, websocket.MessageText, b); err != nil {
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}))
	t.Cleanup(s.Close)
	return s
}

func TestDialRegistersTheDeviceAsItsFirstMessage(t *testing.T) {
	// The server's connection is a command stream until RegisterRunner arrives;
	// sending anything else first would be parsed as a command and rejected.
	srv := newWSServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	l, err := dial(ctx, DialOptions{
		ServerURL: srv.URL,
		Token:     "test-jwt",
		DeviceID:  "laptop",
		Harnesses: []string{"claude"},
	})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer l.Close()

	path := <-srv.gotPath
	for _, want := range []string{
		"/runners/ws", "device_id=laptop",
		fmt.Sprintf("version=%d", TunnelVersion), "harnesses=claude",
	} {
		if !strings.Contains(path, want) {
			t.Errorf("upgrade path missing %q: %s", want, path)
		}
	}
	if proto := <-srv.gotProto; !strings.Contains(proto, "roy-jwt") || !strings.Contains(proto, "test-jwt") {
		t.Errorf("the JWT must ride in the subprotocol slot, got %q", proto)
	}

	var reg Registration
	if err := json.Unmarshal([]byte(<-srv.gotFirst), &reg); err != nil {
		t.Fatalf("first message is not a registration: %v", err)
	}
	if reg.Op != "register_runner" || reg.RunnerID != "laptop" || reg.Version != TunnelVersion {
		t.Fatalf("unexpected registration: %+v", reg)
	}
}

func TestLinkReceivesServerMessages(t *testing.T) {
	srv := newWSServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	l, err := dial(ctx, DialOptions{ServerURL: srv.URL, Token: "t", DeviceID: "d"})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer l.Close()
	<-srv.gotFirst

	srv.toRunner <- Message{Type: MsgOpen, StreamID: 9, Harness: "claude"}
	got, err := l.Recv(ctx)
	if err != nil {
		t.Fatalf("recv: %v", err)
	}
	if got.Type != MsgOpen || got.StreamID != 9 {
		t.Fatalf("unexpected message: %+v", got)
	}
}

func TestDialAcceptsAWebsocketScheme(t *testing.T) {
	// Users will paste wss:// URLs; a runner that only accepted https:// would
	// fail on the obvious input.
	srv := newWSServer(t)
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	l, err := dial(ctx, DialOptions{ServerURL: wsURL, Token: "t", DeviceID: "d"})
	if err != nil {
		t.Fatalf("dial with ws:// scheme: %v", err)
	}
	defer l.Close()
	<-srv.gotFirst
}

func TestDialRejectsAMissingToken(t *testing.T) {
	// Failing here beats a 401 the user has to go read logs to understand.
	_, err := dial(context.Background(), DialOptions{ServerURL: "https://example.invalid", DeviceID: "d"})
	if err == nil {
		t.Fatal("a missing token must be refused before dialling")
	}
	if !strings.Contains(err.Error(), "token") {
		t.Fatalf("the error should name the missing token, got: %v", err)
	}
}
