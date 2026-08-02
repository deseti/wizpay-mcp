package app

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"testing"
)

type fakeLifecycleServer struct {
	serveStarted chan struct{}
	stop         chan struct{}
	serveErr     error
	shutdownErr  error
	shutdownOnce sync.Once
	shutdownSeen bool
}

func newFakeLifecycleServer() *fakeLifecycleServer {
	return &fakeLifecycleServer{serveStarted: make(chan struct{}), stop: make(chan struct{})}
}

func (s *fakeLifecycleServer) Serve(net.Listener) error {
	close(s.serveStarted)
	if s.serveErr != nil {
		return s.serveErr
	}
	<-s.stop
	return http.ErrServerClosed
}

func (s *fakeLifecycleServer) Shutdown(context.Context) error {
	s.shutdownSeen = true
	s.shutdownOnce.Do(func() { close(s.stop) })
	return s.shutdownErr
}

func lifecycleTestLogger(output *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(output, nil))
}

func TestServeUntilCanceledGracefullyShutsDown(t *testing.T) {
	server := newFakeLifecycleServer()
	ctx, cancel := context.WithCancel(context.Background())
	output := &bytes.Buffer{}
	result := make(chan error, 1)
	go func() {
		result <- serveUntilCanceled(ctx, server, nil, lifecycleTestLogger(output))
	}()

	<-server.serveStarted
	cancel()
	if err := <-result; err != nil {
		t.Fatalf("serveUntilCanceled() error = %v", err)
	}
	if !server.shutdownSeen {
		t.Fatal("Shutdown() was not called")
	}
	if !bytes.Contains(output.Bytes(), []byte(`"msg":"server_shutdown"`)) {
		t.Fatalf("shutdown event missing from log: %s", output.String())
	}
}

func TestServeUntilCanceledReturnsServeError(t *testing.T) {
	server := newFakeLifecycleServer()
	server.serveErr = errors.New("serve failed")
	err := serveUntilCanceled(context.Background(), server, nil, lifecycleTestLogger(&bytes.Buffer{}))
	if err == nil || !errors.Is(err, server.serveErr) {
		t.Fatalf("serveUntilCanceled() error = %v, want serve failure", err)
	}
}
