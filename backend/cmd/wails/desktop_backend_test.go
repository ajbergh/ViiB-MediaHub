// Tests loopback startup and shutdown ordering for the Wails backend.
package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"
)

func TestDesktopBackendRejectsOccupiedPortBeforeOpeningDatabase(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	root := t.TempDir()
	if backend, err := startDesktopBackend(listener.Addr().(*net.TCPAddr).Port, root, fstest.MapFS{}); err == nil {
		backend.close()
		t.Fatal("backend started with an occupied port")
	}
	if _, err := os.Stat(filepath.Join(root, "library.db")); !os.IsNotExist(err) {
		t.Fatalf("failed port bind opened the database: %v", err)
	}
}

func TestDesktopBackendStartsOnlyForAPIRequests(t *testing.T) {
	starts, stops := 0, 0
	backend := &desktopBackend{start: func() (*runningDesktopBackend, error) {
		starts++
		return &runningDesktopBackend{proxy: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}), close: func() { stops++ }}, nil
	}}
	for _, tc := range []struct {
		path           string
		status, starts int
	}{
		{"/index.html", http.StatusNotFound, 0},
		{"/api/health", http.StatusNoContent, 1},
		{"/api/health", http.StatusNoContent, 1},
	} {
		record := httptest.NewRecorder()
		backend.ServeHTTP(record, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if record.Code != tc.status || starts != tc.starts {
			t.Fatalf("%s: response %d with %d backend starts", tc.path, record.Code, starts)
		}
	}
	if err := backend.close(); err != nil || stops != 1 {
		t.Fatalf("close error %v, calls %d", err, stops)
	}
}

func TestDesktopBackendCannotStartAfterWindowCloses(t *testing.T) {
	backend := &desktopBackend{start: func() (*runningDesktopBackend, error) {
		t.Fatal("started after window closed")
		return nil, nil
	}}
	if err := backend.close(); err != nil {
		t.Fatal(err)
	}
	if _, err := backend.get(); !errors.Is(err, context.Canceled) {
		t.Fatalf("late callback: %v", err)
	}
}

func TestDesktopBackendServesLoopbackAndAssetProxy(t *testing.T) {
	backend, err := startDesktopBackend(0, t.TempDir(), fstest.MapFS{})
	if err != nil {
		t.Fatal(err)
	}
	defer backend.close()

	client := &http.Client{Timeout: 5 * time.Second}
	response, err := client.Get(backend.serverURL + "/api/health")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("loopback health: %d", response.StatusCode)
	}

	proxy := httptest.NewRecorder()
	backend.proxy.ServeHTTP(proxy, httptest.NewRequest(http.MethodGet, "http://wails.localhost/api/health", nil))
	if proxy.Code != http.StatusOK {
		t.Fatalf("proxied health: %d", proxy.Code)
	}
}
