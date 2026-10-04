//go:build spotify_research

package main

import (
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func fixturePCM() pcmVerification {
	return pcmVerification{FileBytes: 8192, SampleRate: 44100, Channels: 2, DeclaredFrames: 88200, DecodedFrames: 88200, DurationSeconds: 2}
}
func fixtureDownloadOps(t *testing.T, root string) downloadOperations {
	t.Helper()
	oggPath := filepath.Join(root, "probe.ogg")
	mp3Path := filepath.Join(root, "probe.mp3")
	return downloadOperations{
		Initialize: func() error { return nil },
		Download: func(context.Context, string) (string, error) {
			if err := os.MkdirAll(root, 0700); err != nil {
				return "", err
			}
			return oggPath, os.WriteFile(oggPath, make([]byte, 8192), 0600)
		},
		VerifyOgg: func(context.Context, string) (pcmVerification, error) { return fixturePCM(), nil },
		Convert: func(context.Context, string) (string, error) {
			if err := os.WriteFile(mp3Path, make([]byte, 8192), 0600); err != nil {
				return "", err
			}
			return mp3Path, os.Remove(oggPath)
		},
		VerifyMP3: func(context.Context, string) (pcmVerification, error) {
			metrics := fixturePCM()
			metrics.DeclaredFrames = 0
			return metrics, nil
		},
	}
}
func TestDownloadPipelineRecordsCompleteVerification(t *testing.T) {
	root := filepath.Join(t.TempDir(), "download")
	ops := fixtureDownloadOps(t, root)
	report := audioReport{Status: "running", Download: &downloadVerification{}}
	var stages []string
	err := runDownloadPass(context.Background(), ops, "track", root, &report, func() error { stages = append(stages, report.Stage); return nil })
	if err != nil {
		t.Fatal(err)
	}
	d := report.Download
	if !d.SessionAuthenticated || !d.OggDownloaded || d.Ogg == nil || !d.MP3Converted || d.MP3 == nil || !d.SourceRemoved {
		t.Fatalf("%+v", d)
	}
	expected := []string{"audio_session", "audio_download", "ogg_verification", "mp3_conversion", "mp3_verification", "mp3_verification"}
	if len(stages) != len(expected) {
		t.Fatal(stages)
	}
	for i := range expected {
		if stages[i] != expected[i] {
			t.Fatal(stages)
		}
	}
	report.Status = "success"
	report.Stage = "complete"
	if !validAudioReport(report) {
		t.Fatal("valid complete download report rejected")
	}
}
func TestDownloadPipelineDistinguishesFailures(t *testing.T) {
	for _, stage := range []string{"session", "download", "ogg", "convert", "mp3", "duration"} {
		t.Run(stage, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "download")
			ops := fixtureDownloadOps(t, root)
			raw := errors.New("secret upstream account path")
			switch stage {
			case "session":
				ops.Initialize = func() error { return raw }
			case "download":
				ops.Download = func(context.Context, string) (string, error) { return "", raw }
			case "ogg":
				ops.VerifyOgg = func(context.Context, string) (pcmVerification, error) { return pcmVerification{}, raw }
			case "convert":
				ops.Convert = func(context.Context, string) (string, error) { return "", raw }
			case "mp3":
				ops.VerifyMP3 = func(context.Context, string) (pcmVerification, error) { return pcmVerification{}, raw }
			case "duration":
				ops.VerifyMP3 = func(context.Context, string) (pcmVerification, error) {
					metrics := fixturePCM()
					metrics.DeclaredFrames = 0
					metrics.DecodedFrames *= 2
					metrics.DurationSeconds *= 2
					return metrics, nil
				}
			}
			report := audioReport{Status: "running", Download: &downloadVerification{}}
			err := runDownloadPass(context.Background(), ops, "track", root, &report, func() error { return nil })
			if err == nil {
				t.Fatal("failure accepted")
			}
			expected := map[string]string{"session": "audio_session_failed", "download": "audio_download_failed", "ogg": "ogg_verification_failed",
				"convert": "mp3_conversion_failed", "mp3": "mp3_verification_failed", "duration": "mp3_verification_failed"}[stage]
			if code := downloadErrorCode(err, report.Stage); code != expected {
				t.Fatalf("%s stage=%s", code, report.Stage)
			}
			if stage == "convert" {
				if _, err := os.Stat(filepath.Join(root, "probe.ogg")); err != nil {
					t.Fatal("failed conversion lost source")
				}
			}
		})
	}
}
func TestDownloadRejectsEscapedPathsAndLimits(t *testing.T) {
	root := filepath.Join(t.TempDir(), "download")
	os.MkdirAll(root, 0700)
	outside := filepath.Join(t.TempDir(), "outside.ogg")
	os.WriteFile(outside, []byte("not audio"), 0600)
	if !errors.Is(confinedProbeFile(root, outside, ".ogg"), errDownloadPath) {
		t.Fatal("escaped path accepted")
	}
	wrong := filepath.Join(root, "wrong.mp3")
	os.WriteFile(wrong, []byte("not audio"), 0600)
	if !errors.Is(confinedProbeFile(root, wrong, ".ogg"), errDownloadPath) {
		t.Fatal("wrong extension accepted")
	}
	huge := filepath.Join(root, "huge.ogg")
	file, err := os.Create(huge)
	if err != nil {
		t.Fatal(err)
	}
	err = file.Truncate(maxProbeAudioFileBytes + 1)
	file.Close()
	if err != nil {
		t.Fatal(err)
	}
	if !errors.Is(confinedProbeFile(root, huge, ".ogg"), errDownloadLimit) {
		t.Fatal("oversized file accepted")
	}
	ops := fixtureDownloadOps(t, root)
	ops.Download = func(context.Context, string) (string, error) { return outside, nil }
	report := audioReport{Status: "running", Download: &downloadVerification{}}
	err = runDownloadPass(context.Background(), ops, "track", root, &report, func() error { return nil })
	if !errors.Is(err, errDownloadPath) || report.Download.OggDownloaded {
		t.Fatal("escaped output was validated")
	}
}
func TestDownloadVerificationBoundsAndModeGuards(t *testing.T) {
	if err := validateDownloadOptions(true, 0, false, false, false); err != nil {
		t.Fatal(err)
	}
	for _, flags := range []struct {
		bytes                  int
		web, features, compare bool
	}{{4096, false, false, false}, {0, true, false, false}, {0, false, true, false}, {0, false, false, true}} {
		if validateDownloadOptions(true, flags.bytes, flags.web, flags.features, flags.compare) == nil {
			t.Fatal("mixed modes accepted")
		}
	}
	for _, kind := range []string{"nan", "duration", "declared", "geometry", "bytes"} {
		metrics := fixturePCM()
		switch kind {
		case "nan":
			metrics.DurationSeconds = math.NaN()
		case "duration":
			metrics.DurationSeconds = maxProbeAudioSeconds + 1
		case "declared":
			metrics.DeclaredFrames++
		case "geometry":
			metrics.Channels = 8
		case "bytes":
			metrics.FileBytes = maxProbeAudioFileBytes + 1
		}
		if validPCMVerification(metrics) {
			t.Fatalf("invalid %s metrics accepted", kind)
		}
	}
}
func TestDownloadDecodersRejectCorruptAudioAndCancellation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "corrupt.ogg")
	os.WriteFile(path, []byte("not a valid audio stream"), 0600)
	if _, err := verifyProbeOgg(context.Background(), path); err == nil {
		t.Fatal("corrupt Ogg accepted")
	}
	if _, err := verifyProbeMP3(context.Background(), path); err == nil {
		t.Fatal("corrupt MP3 accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := verifyProbeOgg(ctx, path); !errors.Is(err, context.Canceled) {
		t.Fatalf("Ogg cancel %v", err)
	}
	if _, err := verifyProbeMP3(ctx, path); !errors.Is(err, context.Canceled) {
		t.Fatalf("MP3 cancel %v", err)
	}
}
