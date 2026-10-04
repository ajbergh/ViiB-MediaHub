package main

import (
	"context"
	"net"
	"net/http"
	"time"
)

// Long-lived event streams inherit a server-owned request lifetime. Cancel it
// before Shutdown so active handlers can return instead of waiting for clients.
func newHTTPServer(handler http.Handler) (*http.Server, context.CancelFunc) {
	requests, cancel := context.WithCancel(context.Background())
	return &http.Server{
		Handler:      handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0,
		IdleTimeout:  120 * time.Second,
		BaseContext:  func(net.Listener) context.Context { return requests },
	}, cancel
}
