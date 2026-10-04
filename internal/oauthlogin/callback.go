package oauthlogin

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
)

// callbackResult is what the authorize redirect carried back: either code+state
// on approval, or errorParam (+ whatever state rode along) on refusal.
type callbackResult struct {
	code       string
	state      string
	errorParam string
}

// callbackServer is the loopback HTTP listener that catches the browser's
// redirect after the authorize screen. redirectURI is fixed at bind time —
// before the authorize URL is built — because the port is picked by the OS
// and must be known to register the client and construct that URL.
type callbackServer struct {
	redirectURI string
	srv         *http.Server
	ln          net.Listener
	result      chan callbackResult
}

// listenForCallback binds 127.0.0.1:0 (an OS-assigned free port) and starts
// serving immediately, so a caller can read redirectURI right away.
func listenForCallback() (*callbackServer, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	cb := &callbackServer{
		redirectURI: fmt.Sprintf("http://127.0.0.1:%d/callback", ln.Addr().(*net.TCPAddr).Port),
		ln:          ln,
		result:      make(chan callbackResult, 1),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", cb.handle)
	cb.srv = &http.Server{Handler: mux}
	go func() { _ = cb.srv.Serve(ln) }()
	return cb, nil
}

func (cb *callbackServer) handle(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	select {
	case cb.result <- callbackResult{code: q.Get("code"), state: q.Get("state"), errorParam: q.Get("error")}:
	default:
		// A second hit (a reload, a retry) after the first is already queued —
		// the one the login attempt is waiting for was already delivered.
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(`<!doctype html><html><body>You can close this tab and return to the terminal.</body></html>`))
}

// wait blocks until the browser visits the callback or ctx is done, whichever
// comes first.
func (cb *callbackServer) wait(ctx context.Context) (callbackResult, error) {
	select {
	case r := <-cb.result:
		return r, nil
	case <-ctx.Done():
		return callbackResult{}, errors.New("timed out waiting for the browser to complete sign-in")
	}
}

// Close shuts down the listener. Safe to call once the login attempt is done,
// successful or not.
func (cb *callbackServer) Close() error {
	return cb.srv.Close()
}
