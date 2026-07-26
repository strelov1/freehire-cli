package runner

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
	// Where session activity is reported. A runner that prints nothing between
	// "connected" and a crash is indistinguishable from an idle one, so every
	// request from the server is announced.
	log io.Writer
	// verbose adds a line per protocol frame. Off by default: a single turn is
	// hundreds of frames, which buries the events that matter.
	verbose bool

	mu   sync.Mutex
	open map[uint64]process
	wg   sync.WaitGroup
}

func newSessions(l link, spawn spawnFunc) *sessions {
	return &sessions{link: l, spawn: spawn, log: io.Discard, open: map[uint64]process{}}
}

func (s *sessions) logf(format string, args ...any) {
	if s.log == nil {
		return
	}
	fmt.Fprintf(s.log, format+"\n", args...)
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
	s.logf("session %d: server asked for harness %q", msg.StreamID, msg.Harness)
	harness, ok := LookupHarness(msg.Harness)
	if !ok {
		// Refused before a process exists — this is the allowlist doing its job.
		s.logf("session %d: refused — %q is not a harness this runner offers",
			msg.StreamID, msg.Harness)
		_ = s.link.Send(ctx, OpenFailed(msg.StreamID, "unknown harness: "+msg.Harness))
		return
	}
	dir, err := sessionDir(msg.StreamID)
	if err != nil {
		_ = s.link.Send(ctx, OpenFailed(msg.StreamID, "preparing a working directory: "+err.Error()))
		return
	}
	harness.Dir = dir

	proc, err := s.spawn(harness, msg.Env)
	if err != nil {
		// The user's own reason, not a category: "not found in $PATH" is
		// actionable, "spawn failed" is not.
		s.logf("session %d: could not start %s — %v", msg.StreamID, harness.Command, err)
		_ = s.link.Send(ctx, OpenFailed(msg.StreamID, err.Error()))
		return
	}
	s.logf("session %d: started %s in %s", msg.StreamID, harness.Command, dir)

	s.mu.Lock()
	s.open[msg.StreamID] = proc
	s.mu.Unlock()

	if err := s.link.Send(ctx, Opened(msg.StreamID, dir)); err != nil {
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
	frames := 0
	for line := range proc.Lines() {
		if s.verbose {
			s.logf("session %d: → %s", stream, truncate(line, 160))
		}
		if err := s.link.Send(ctx, Frame(stream, line)); err != nil {
			return
		}
		frames++
	}
	code := proc.Wait()
	s.logf("session %d: harness exited (code %d) after %d messages", stream, code, frames)

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
		s.logf("session %d: server closed it; stopping the harness", stream)
		proc.Kill()
	}
}

// truncate keeps a log line readable without hiding that it was cut.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// sessionDir is where a session's harness works: a scratch directory under the
// user's home, one per stream.
//
// It is scratch on purpose. Everything durable — the CV, the vacancy, the
// conversation — lives on the server and is reached through the `freehire`
// CLI, so nothing here needs to survive the session.
func sessionDir(stream uint64) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, dirName, "runner", "sessions", fmt.Sprint(stream))
	if err := os.MkdirAll(filepath.Join(dir, ".claude"), 0o700); err != nil {
		return "", err
	}
	// The harness reads this from its working directory and confines itself
	// with it. Without it the agent cannot run even `freehire`, because the
	// server deliberately answers every permission request with "deny" —
	// policy belongs to the machine the agent runs on.
	self, err := os.Executable()
	if err != nil {
		self = "freehire"
	}
	settings := filepath.Join(dir, ".claude", "settings.json")
	if err := os.WriteFile(settings, []byte(sessionSettings(self)), 0o600); err != nil {
		return "", fmt.Errorf("writing %s: %w", settings, err)
	}
	return dir, nil
}

func (s *sessions) shutdown() {
	s.mu.Lock()
	procs := make([]process, 0, len(s.open))
	for id, p := range s.open {
		procs = append(procs, p)
		delete(s.open, id)
	}
	s.mu.Unlock()
	if len(procs) > 0 {
		s.logf("connection lost; stopping %d harness(es)", len(procs))
	}
	for _, p := range procs {
		p.Kill()
	}
	s.wg.Wait()
}
