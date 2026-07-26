package runner

import (
	"context"
	"sync"
)

// link is the tunnel as this file needs it: send a message, receive a message.
// Narrow on purpose — the session logic has no business knowing it is a
// WebSocket, and tests drive it without one.
type link interface {
	Send(ctx context.Context, m Message) error
	Recv(ctx context.Context) (Message, error)
}

// process is a running harness: write it a line, read its lines, wait for it to
// exit, kill it.
type process interface {
	WriteLine(s string) error
	Lines() <-chan string
	Wait() int
	Kill()
}

// spawnFunc starts a harness. Injected so the session logic can be tested
// without real processes.
type spawnFunc func(h Harness, env map[string]string) (process, error)

// sessions multiplexes every harness on this device over one tunnel, keyed by
// the stream id the server assigns.
type sessions struct {
	link  link
	spawn spawnFunc

	mu   sync.Mutex
	open map[uint64]process
	wg   sync.WaitGroup
}

func newSessions(l link, spawn spawnFunc) *sessions {
	return &sessions{link: l, spawn: spawn, open: map[uint64]process{}}
}

// run serves the tunnel until it fails or ctx is cancelled. On the way out it
// kills every harness: the server can no longer reach them, and they are
// spending the user's model quota.
func (s *sessions) run(ctx context.Context) error {
	defer s.shutdown()
	for {
		msg, err := s.link.Recv(ctx)
		if err != nil {
			return err
		}
		switch msg.Type {
		case MsgOpen:
			s.handleOpen(ctx, msg)
		case MsgFrame:
			s.handleFrame(msg)
		case MsgClose:
			s.handleClose(msg.StreamID)
		}
		// Anything else is a message only this side sends; ignoring it keeps a
		// chatty or newer server from killing the connection.
	}
}

func (s *sessions) handleOpen(ctx context.Context, msg Message) {
	harness, ok := LookupHarness(msg.Harness)
	if !ok {
		// Refused before a process exists — this is the allowlist doing its job.
		_ = s.link.Send(ctx, OpenFailed(msg.StreamID, "unknown harness: "+msg.Harness))
		return
	}
	proc, err := s.spawn(harness, msg.Env)
	if err != nil {
		// The user's own reason, not a category: "not found in $PATH" is
		// actionable, "spawn failed" is not.
		_ = s.link.Send(ctx, OpenFailed(msg.StreamID, err.Error()))
		return
	}

	s.mu.Lock()
	s.open[msg.StreamID] = proc
	s.mu.Unlock()

	if err := s.link.Send(ctx, Opened(msg.StreamID)); err != nil {
		s.handleClose(msg.StreamID)
		return
	}
	s.wg.Add(1)
	go s.pumpOut(ctx, msg.StreamID, proc)
}

// pumpOut forwards the harness's output and reports its exit. The Closed
// message is what lets the server end an in-flight turn instead of waiting on a
// stream that will never produce again.
func (s *sessions) pumpOut(ctx context.Context, stream uint64, proc process) {
	defer s.wg.Done()
	for line := range proc.Lines() {
		if err := s.link.Send(ctx, Frame(stream, line)); err != nil {
			return
		}
	}
	code := proc.Wait()

	s.mu.Lock()
	delete(s.open, stream)
	s.mu.Unlock()

	_ = s.link.Send(ctx, Closed(stream, code))
}

func (s *sessions) handleFrame(msg Message) {
	s.mu.Lock()
	proc, ok := s.open[msg.StreamID]
	s.mu.Unlock()
	if !ok {
		// The stream closed while the server was still writing to it. Normal,
		// not an error.
		return
	}
	_ = proc.WriteLine(msg.Data)
}

func (s *sessions) handleClose(stream uint64) {
	s.mu.Lock()
	proc, ok := s.open[stream]
	delete(s.open, stream)
	s.mu.Unlock()
	if ok {
		proc.Kill()
	}
}

func (s *sessions) shutdown() {
	s.mu.Lock()
	procs := make([]process, 0, len(s.open))
	for id, p := range s.open {
		procs = append(procs, p)
		delete(s.open, id)
	}
	s.mu.Unlock()
	for _, p := range procs {
		p.Kill()
	}
	s.wg.Wait()
}
