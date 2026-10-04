// Tests loopback server startup and cancellation of active requests before shutdown.
package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestHTTPShutdownCancelsOpenEventStream(t *testing.T) {
	entered := make(chan struct{})
	exited := make(chan struct{})
	server, cancel := newHTTPServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, ": connected\n\n")
		w.(http.Flusher).Flush()
		close(entered)
		<-r.Context().Done()
		close(exited)
	}))
	defer cancel()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	served := make(chan error, 1)
	go func() { served <- server.Serve(listener) }()
	response, err := http.Get("http://" + listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	<-entered
	cancel()
	ctx, stop := context.WithTimeout(context.Background(), 2*time.Second)
	defer stop()
	if err := server.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-exited:
	case <-ctx.Done():
		t.Fatal("event stream did not retire")
	}
	if err := <-served; err != http.ErrServerClosed {
		t.Fatalf("serve: %v", err)
	}
}
