//go:build spotify_research

// Tests running bounded librespot audio probes with cancellation, cleanup, and watchdog diagnostics.
package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type fixtureAudioBackend struct {
	initializeErr, openErr error
	stream                 io.ReadCloser
	opened                 bool
}

func (b *fixtureAudioBackend) Initialize(string) error { return b.initializeErr }
func (b *fixtureAudioBackend) Open(context.Context, string) (io.ReadCloser, error) {
	b.opened = true
	return b.stream, b.openErr
}

type fixtureAudioStream struct {
	*bytes.Reader
	closed bool
}

func (s *fixtureAudioStream) Close() error { s.closed = true; return nil }

func TestAudioOptionsAreExplicitAndBounded(t *testing.T) {
	for _, n := range []int{-1, 1, 4095, 16385} {
		if validateAudioOptions(n, false, false) == nil {
			t.Fatalf("accepted %d", n)
		}
	}
	for _, n := range []int{0, 4096, 16384} {
		if err := validateAudioOptions(n, false, false); err != nil {
			t.Fatal(err)
		}
	}
	if validateAudioOptions(4096, true, false) == nil || validateAudioOptions(4096, false, true) == nil {
		t.Fatal("mixed probe modes accepted")
	}
}
func TestAudioReadRecordsMilestonesAndClosesStream(t *testing.T) {
	data := make([]byte, 20000)
	copy(data, "OggS")
	stream := &fixtureAudioStream{Reader: bytes.NewReader(data)}
	backend := &fixtureAudioBackend{stream: stream}
	report := audioReport{Status: "running"}
	var stages []string
	err := audioReadPass(context.Background(), backend, "not-a-live-token", "track", 4096, &report.Initial, &report,
		func() error { stages = append(stages, report.Stage); return nil })
	if err != nil || !report.Initial.SessionAuthenticated || !report.Initial.StreamOpened ||
		report.Initial.BytesRead != 4096 || !report.Initial.OggHeaderVerified || !stream.closed {
		t.Fatalf("pass %+v %v", report, err)
	}
	if stream.Len() != 20000-4096 {
		t.Fatal("read exceeded cap")
	}
	if len(stages) != 4 || stages[0] != "audio_session" || stages[1] != "audio_stream" || stages[2] != "audio_read" {
		t.Fatal(stages)
	}
}
func TestAudioFailuresDistinguishLoginStreamAndRead(t *testing.T) {
	for _, kind := range []string{"login", "stream", "read", "header"} {
		t.Run(kind, func(t *testing.T) {
			backend := &fixtureAudioBackend{}
			switch kind {
			case "login":
				backend.initializeErr = errors.New("raw secret")
			case "stream":
				backend.openErr = errors.New("raw secret")
			case "read":
				backend.stream = &fixtureAudioStream{Reader: bytes.NewReader([]byte("OggS"))}
			case "header":
				backend.stream = &fixtureAudioStream{Reader: bytes.NewReader(make([]byte, 4096))}
			}
			report := audioReport{Status: "running"}
			err := audioReadPass(context.Background(), backend, "secret", "id", 4096, &report.Initial, &report, func() error { return nil })
			if err == nil {
				t.Fatal("failure accepted")
			}
			expected := map[string]string{"login": "audio_session_failed", "stream": "audio_stream_failed", "read": "audio_read_failed", "header": "ogg_header_missing"}[kind]
			if code := audioErrorCode(err, report.Stage); code != expected {
				t.Fatalf("code=%s stage=%s", code, report.Stage)
			}
			if kind == "login" && (report.Initial.SessionAuthenticated || backend.opened) {
				t.Fatal("failed login claimed streaming")
			}
		})
	}
}
func TestAudioWorkerHelper(t *testing.T) {
	if os.Getenv("VIIB_AUDIO_TEST_HELPER") != "1" {
		return
	}
	path := os.Getenv(audioWorkerReportEnv)
	report := audioReport{Status: "running", Stage: "audio_session", AuthenticationVerified: true}
	if err := writeAudioMilestones(path, report); err != nil {
		os.Exit(2)
	}
	// Must not leak raw dependency stdout/stderr through the parent.
	os.Stdout.WriteString("secret token cookie raw upstream output")
	os.Stderr.WriteString("secret token cookie raw upstream output")
	if os.Getenv("VIIB_AUDIO_TEST_HANG") == "1" {
		time.Sleep(30 * time.Second)
		os.Exit(0)
	}
	report.Status = "success"
	report.Stage = "complete"
	report.Initial = audioPass{SessionAuthenticated: true, StreamOpened: true, BytesRead: 4096, OggHeaderVerified: true}
	if err := writeAudioMilestones(path, report); err != nil {
		os.Exit(2)
	}
	os.Exit(0)
}
func TestAudioWatchdogTerminatesHungDependency(t *testing.T) {
	path := filepath.Join(t.TempDir(), "milestones.json")
	env := append(os.Environ(), "VIIB_AUDIO_TEST_HELPER=1", "VIIB_AUDIO_TEST_HANG=1", audioWorkerReportEnv+"="+path)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	report := executeAudioWorker(context.Background(), executable, []string{"-test.run=^TestAudioWorkerHelper$"}, env, path, time.Second)
	if report.Status != "failed" || report.Code != "timeout" || !report.AuthenticationVerified || report.Stage != "audio_session" {
		t.Fatalf("watchdog %+v", report)
	}
	if time.Since(start) > 5*time.Second {
		t.Fatal("watchdog did not bound process")
	}
}
func TestAudioWorkerReturnsOnlySafeReport(t *testing.T) {
	path := filepath.Join(t.TempDir(), "milestones.json")
	env := append(os.Environ(), "VIIB_AUDIO_TEST_HELPER=1", audioWorkerReportEnv+"="+path)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	report := executeAudioWorker(context.Background(), executable, []string{"-test.run=^TestAudioWorkerHelper$"}, env, path, 5*time.Second)
	if report.Status != "success" || report.Initial.BytesRead != 4096 {
		t.Fatalf("report %+v", report)
	}
	if validAudioReport(audioReport{Status: "failed", Stage: "secret token", Code: "secret cookie"}) {
		t.Fatal("arbitrary output accepted")
	}
}
