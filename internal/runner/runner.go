package runner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"strings"
	"time"
)

const (
	baseBackoff = 500 * time.Millisecond
	maxBackoff  = 30 * time.Second
)

// errFatal marks a failure that retrying cannot fix — a rejected token, a
// protocol version the server refuses. Retrying those burns the server's rate
// limit and hides the real problem from the user.
type errFatal struct{ err error }

func (e errFatal) Error() string { return e.err.Error() }
func (e errFatal) Unwrap() error { return e.err }

// backoff grows with each consecutive failure and stops at a cap, with jitter
// so a fleet of runners does not reconnect in lockstep after an outage.
func backoff(attempt int) time.Duration {
	d := baseBackoff << min(attempt, 6)
	if d > maxBackoff {
		d = maxBackoff
	}
	// Up to 20% jitter, always positive.
	return d - time.Duration(rand.Int64N(int64(d/5)))
}

// Options configures a running device.
type Options struct {
	ServerURL string
	Token     string
	DeviceID  string
	// Out receives human-readable status. Nil discards it.
	Out io.Writer
}

// Run connects and serves until ctx is cancelled, reconnecting on failure.
//
// The connection is long-lived and mostly idle: it exists so the server can
// reach this machine when a session needs it. Sessions come and go over it.
func Run(ctx context.Context, opt Options) error {
	if opt.Out == nil {
		opt.Out = io.Discard
	}
	harnesses := SupportedHarnesses()
	fmt.Fprintf(opt.Out, "device %s offering %v\n", opt.DeviceID, harnesses)

	return serveLoop(ctx, func(ctx context.Context) error {
		l, err := dial(ctx, DialOptions{
			ServerURL: opt.ServerURL,
			Token:     opt.Token,
			DeviceID:  opt.DeviceID,
			Harnesses: harnesses,
		})
		if err != nil {
			if isAuthFailure(err) {
				return errFatal{err}
			}
			// Say why out loud: a runner that retries in silence looks
			// identical to one that is connected, and the user has no other
			// way to tell.
			fmt.Fprintln(opt.Out, "connect failed:", err)
			return err
		}
		defer l.Close()
		fmt.Fprintf(opt.Out, "connected to %s — waiting for sessions\n", opt.ServerURL)

		err = newSessions(l, startProcess).run(ctx)
		fmt.Fprintln(opt.Out, "disconnected:", err)
		return err
	}, backoff)
}

// serveLoop runs connect until it returns a fatal error or ctx ends, waiting
// between attempts. Split out from Run so the retry policy is testable without
// a server.
func serveLoop(
	ctx context.Context,
	connect func(context.Context) error,
	delay func(attempt int) time.Duration,
) error {
	for attempt := 0; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := connect(ctx)
		if err == nil {
			// A clean return means the server closed the tunnel deliberately;
			// treat it as a reconnectable event, not a reason to exit.
			err = errors.New("server closed the connection")
		}
		var fatal errFatal
		if errors.As(err, &fatal) {
			return err
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		select {
		case <-time.After(delay(attempt)):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// isAuthFailure spots a rejected credential in the dial error. The WebSocket
// handshake surfaces it as an HTTP status, so matching the text is the
// available signal.
func isAuthFailure(err error) bool {
	s := err.Error()
	return strings.Contains(s, "401") || strings.Contains(s, "403")
}

// ResolveToken returns the runner's session JWT: the flag wins, then the
// environment.
//
// There is deliberately no third source. The token is short-lived and minted
// per device, so persisting it would create a stale credential on disk for no
// gain.
func ResolveToken(flagValue string) string {
	if flagValue != "" {
		return flagValue
	}
	return os.Getenv("ROY_RUNNER_TOKEN")
}
