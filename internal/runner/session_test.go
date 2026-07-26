package runner

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// --- test doubles -----------------------------------------------------------

// fakeLink stands in for the WebSocket: records what the runner sent and lets a
// test feed it what the server would say.
type fakeLink struct {
	mu   sync.Mutex
	sent []Message
	in   chan Message
}

func newFakeLink() *fakeLink {
	return &fakeLink{in: make(chan Message, 16)}
}

func (f *fakeLink) Send(_ context.Context, m Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, m)
	return nil
}

func (f *fakeLink) Recv(ctx context.Context) (Message, error) {
	select {
	case m, ok := <-f.in:
		if !ok {
			return Message{}, errors.New("closed")
		}
		return m, nil
	case <-ctx.Done():
		return Message{}, ctx.Err()
	}
}

// waitFor polls the recorded messages until pred is satisfied, so tests do not
// depend on goroutine scheduling.
func (f *fakeLink) waitFor(t *testing.T, what string, pred func([]Message) bool) []Message {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		f.mu.Lock()
		got := append([]Message(nil), f.sent...)
		f.mu.Unlock()
		if pred(got) {
			return got
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s; sent so far: %+v", what, got)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// fakeProc stands in for a harness process.
type fakeProc struct {
	mu      sync.Mutex
	written []string
	out     chan string
	killed  bool
	exit    chan int
}

func newFakeProc() *fakeProc {
	return &fakeProc{out: make(chan string, 8), exit: make(chan int, 1)}
}

func (p *fakeProc) WriteLine(s string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.written = append(p.written, s)
	return nil
}
func (p *fakeProc) Lines() <-chan string { return p.out }
func (p *fakeProc) Wait() int            { return <-p.exit }
func (p *fakeProc) Kill() {
	p.mu.Lock()
	p.killed = true
	p.mu.Unlock()
	close(p.out)
	select {
	case p.exit <- 137:
	default:
	}
}

// --- tests ------------------------------------------------------------------

func TestOpenStartsTheHarnessAndConfirms(t *testing.T) {
	link := newFakeLink()
	proc := newFakeProc()
	var gotHarness Harness
	var gotEnv map[string]string

	r := newSessions(link, func(h Harness, env map[string]string) (process, error) {
		gotHarness, gotEnv = h, env
		return proc, nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go r.run(ctx)

	link.in <- Message{
		Type: MsgOpen, StreamID: 1, Harness: "claude",
		Env: map[string]string{"ROY_SESSION_ID": "sess-1"},
	}

	link.waitFor(t, "opened", func(ms []Message) bool {
		return len(ms) > 0 && ms[0].Type == MsgOpened && ms[0].StreamID == 1
	})
	if gotHarness.Command != "claude-code-acp" {
		t.Fatalf("harness = %+v", gotHarness)
	}
	if gotEnv["ROY_SESSION_ID"] != "sess-1" {
		t.Fatalf("env not passed through: %+v", gotEnv)
	}
}

func TestOpenOfAnUnknownHarnessIsRefusedWithoutSpawning(t *testing.T) {
	link := newFakeLink()
	spawned := false
	r := newSessions(link, func(Harness, map[string]string) (process, error) {
		spawned = true
		return newFakeProc(), nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go r.run(ctx)

	link.in <- Message{Type: MsgOpen, StreamID: 2, Harness: "bash"}

	got := link.waitFor(t, "open_failed", func(ms []Message) bool {
		return len(ms) > 0 && ms[0].Type == MsgOpenFailed
	})
	if spawned {
		t.Fatal("an unknown harness must not start a process")
	}
	if got[0].Reason == "" {
		t.Fatal("the refusal must carry a reason")
	}
}

func TestASpawnFailureIsReportedWithItsReason(t *testing.T) {
	// "claude-code-acp is not installed" has to reach the user; it must not
	// look like a timeout on the server.
	link := newFakeLink()
	r := newSessions(link, func(Harness, map[string]string) (process, error) {
		return nil, errors.New("exec: \"claude-code-acp\": executable file not found in $PATH")
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go r.run(ctx)

	link.in <- Message{Type: MsgOpen, StreamID: 3, Harness: "claude"}

	got := link.waitFor(t, "open_failed", func(ms []Message) bool {
		return len(ms) > 0 && ms[0].Type == MsgOpenFailed
	})
	if got[0].Reason == "" || got[0].StreamID != 3 {
		t.Fatalf("unexpected refusal: %+v", got[0])
	}
}

func TestFramesFlowBothWays(t *testing.T) {
	link := newFakeLink()
	proc := newFakeProc()
	r := newSessions(link, func(Harness, map[string]string) (process, error) {
		return proc, nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go r.run(ctx)

	link.in <- Message{Type: MsgOpen, StreamID: 4, Harness: "claude"}
	link.waitFor(t, "opened", func(ms []Message) bool { return len(ms) > 0 })

	// server → harness
	link.in <- Message{Type: MsgFrame, StreamID: 4, Data: `{"jsonrpc":"2.0"}`}
	deadline := time.Now().Add(2 * time.Second)
	for {
		proc.mu.Lock()
		n := len(proc.written)
		proc.mu.Unlock()
		if n > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("frame never reached the harness")
		}
		time.Sleep(5 * time.Millisecond)
	}

	// harness → server
	proc.out <- `{"result":"ok"}`
	got := link.waitFor(t, "frame back", func(ms []Message) bool {
		for _, m := range ms {
			if m.Type == MsgFrame && m.Data == `{"result":"ok"}` {
				return true
			}
		}
		return false
	})
	for _, m := range got {
		if m.Type == MsgFrame && m.StreamID != 4 {
			t.Fatalf("frame carried the wrong stream: %+v", m)
		}
	}
}

func TestHarnessExitIsReportedAsClosed(t *testing.T) {
	// Without this the server would wait on a stream that never ends and the
	// turn would hang instead of failing.
	link := newFakeLink()
	proc := newFakeProc()
	r := newSessions(link, func(Harness, map[string]string) (process, error) {
		return proc, nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go r.run(ctx)

	link.in <- Message{Type: MsgOpen, StreamID: 5, Harness: "claude"}
	link.waitFor(t, "opened", func(ms []Message) bool { return len(ms) > 0 })

	close(proc.out)
	proc.exit <- 1

	link.waitFor(t, "closed", func(ms []Message) bool {
		for _, m := range ms {
			if m.Type == MsgClosed && m.StreamID == 5 {
				return true
			}
		}
		return false
	})
}

func TestCloseKillsTheHarness(t *testing.T) {
	link := newFakeLink()
	proc := newFakeProc()
	r := newSessions(link, func(Harness, map[string]string) (process, error) {
		return proc, nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go r.run(ctx)

	link.in <- Message{Type: MsgOpen, StreamID: 6, Harness: "claude"}
	link.waitFor(t, "opened", func(ms []Message) bool { return len(ms) > 0 })

	link.in <- Message{Type: MsgClose, StreamID: 6}

	deadline := time.Now().Add(2 * time.Second)
	for {
		proc.mu.Lock()
		killed := proc.killed
		proc.mu.Unlock()
		if killed {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("close must kill the harness")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestLosingTheLinkKillsEveryHarness(t *testing.T) {
	// The server cannot reach these processes any more, and they hold the
	// user's model quota. Orphaning them would leak both.
	link := newFakeLink()
	proc := newFakeProc()
	r := newSessions(link, func(Harness, map[string]string) (process, error) {
		return proc, nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	go r.run(ctx)

	link.in <- Message{Type: MsgOpen, StreamID: 7, Harness: "claude"}
	link.waitFor(t, "opened", func(ms []Message) bool { return len(ms) > 0 })

	cancel() // the link drops

	deadline := time.Now().Add(2 * time.Second)
	for {
		proc.mu.Lock()
		killed := proc.killed
		proc.mu.Unlock()
		if killed {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("a lost link must tear down running harnesses")
		}
		time.Sleep(5 * time.Millisecond)
	}
}
