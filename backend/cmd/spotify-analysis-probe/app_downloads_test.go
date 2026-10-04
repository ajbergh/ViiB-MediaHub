//go:build spotify_research

package main

import (
	"context"
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestAppDownloadVerifierRestrictsRootAndRejectsCorruption(t *testing.T) {
	if err := verifyAppDownloadArtifacts(context.Background(), t.TempDir(), io.Discard); !errors.Is(err, errDownloadPath) {
		t.Fatal("arbitrary directory accepted")
	}
	root, err := os.MkdirTemp("", "viib-cookie-app-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(root) })
	directory := filepath.Join(root, "data", "spotify_downloads")
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	if err := verifyAppDownloadArtifacts(context.Background(), root, io.Discard); err == nil {
		t.Fatal("empty output accepted")
	}
	corrupt := filepath.Join(directory, "corrupt.ogg")
	if err := os.WriteFile(corrupt, []byte("not audio"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := verifyAppDownloadArtifacts(context.Background(), root, io.Discard); !errors.Is(err, errIncompleteAudio) {
		t.Fatal("corrupt production artifact accepted")
	}
}

func TestOfflineAppVerifierDoesNotRequireTrackInput(t *testing.T) {
	root, err := os.MkdirTemp("", "viib-cookie-app-cli-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(root) })
	directory := filepath.Join(root, "data", "spotify_downloads")
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "corrupt.ogg"), []byte("not audio"), 0600); err != nil {
		t.Fatal(err)
	}
	savedFlags, savedArgs := flag.CommandLine, os.Args
	t.Cleanup(func() { flag.CommandLine = savedFlags; os.Args = savedArgs })
	flag.CommandLine = flag.NewFlagSet("offline-verifier", flag.ContinueOnError)
	os.Args = []string{"offline-verifier", "--enable", "--verify-app-downloads", root}
	err = run(context.Background())
	if !errors.Is(err, errIncompleteAudio) {
		t.Fatalf("offline mode did not reach audio verifier without track ID: %v", err)
	}
}
