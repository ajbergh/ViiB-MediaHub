//go:build spotify_research

// Runs bounded librespot audio probes with cancellation, cleanup, and watchdog diagnostics.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/spotify"
	"github.com/ajbergh/viib-mediahub/internal/spotify/auth"
)

const audioWorkerReportEnv = "VIIB_SPOTIFY_AUDIO_WORKER_REPORT"
const audioProbeTimeout = 75 * time.Second

var errAudioProbeFailed = errors.New("audio compatibility probe failed")

// These are milestones, not claims of complete playback/download compatibility.
type audioPass struct {
	SessionAuthenticated bool `json:"sessionAuthenticated"`
	StreamOpened         bool `json:"streamOpened"`
	BytesRead            int  `json:"bytesRead"`
	OggHeaderVerified    bool `json:"oggHeaderVerified"`
}
type audioReport struct {
	Download               *downloadVerification `json:"download,omitempty"`
	Status                 string                `json:"status"`
	Stage                  string                `json:"stage"`
	Code                   string                `json:"code,omitempty"`
	AuthenticationVerified bool                  `json:"authenticationVerified"`
	RenewalVerified        bool                  `json:"renewalVerified"`
	Initial                audioPass             `json:"initial"`
	AfterRenewal           *audioPass            `json:"afterRenewal,omitempty"`
}

func validateAudioOptions(bytes int, compare, features bool) error {
	if bytes == 0 {
		return nil
	}
	if bytes < 4096 || bytes > 16384 || compare || features {
		return errors.New("audio probe requires 4096..16384 bytes and no analysis flags")
	}
	return nil
}

func audioMode(ctx context.Context, id string, bytes int, renew bool, out io.Writer) error {
	if path := os.Getenv(audioWorkerReportEnv); path != "" {
		_ = os.Unsetenv(audioWorkerReportEnv)
		return audioWorker(ctx, id, bytes, renew, path)
	}
	executable, err := os.Executable()
	if err != nil {
		return errAudioProbeFailed
	}
	directory, err := os.MkdirTemp("", "viib-spotify-audio-probe-")
	if err != nil {
		return errAudioProbeFailed
	}
	defer os.RemoveAll(directory)
	path := filepath.Join(directory, "milestones.json")
	args := []string{"--enable", "--track-id", id, "--stream-read-bytes", strconv.Itoa(bytes), "--renew=" + strconv.FormatBool(renew)}
	// Cookie stays in the child's environment, never its arguments or report file.
	environment := append(os.Environ(), audioWorkerReportEnv+"="+path)
	_ = os.Unsetenv("VIIB_SPOTIFY_SP_DC")
	report := executeAudioWorker(ctx, executable, args, environment, path, audioProbeTimeout)
	if err = json.NewEncoder(out).Encode(report); err != nil {
		return err
	}
	if report.Status != "success" {
		return errAudioProbeFailed
	}
	return nil
}
func executeAudioWorker(ctx context.Context, executable string, args, environment []string, path string, timeout time.Duration) audioReport {
	workCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	command := exec.CommandContext(workCtx, executable, args...)
	command.Env = environment
	// Dependency logs and panic output may contain account/endpoint details.
	// Discard them at the process boundary; never capture/persist raw output.
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	command.WaitDelay = time.Second
	runErr := command.Run()
	report := audioReport{Status: "failed", Stage: "audio_worker", Code: "worker_failed"}
	file, err := os.Open(path)
	if err == nil {
		data, readErr := io.ReadAll(io.LimitReader(file, 16385))
		file.Close()
		if readErr == nil && len(data) <= 16384 {
			var candidate audioReport
			if json.Unmarshal(data, &candidate) == nil && validAudioReport(candidate) {
				report = candidate
			}
		}
	}
	if workCtx.Err() != nil {
		report.Status = "failed"
		report.Code = "timeout"
		if errors.Is(ctx.Err(), context.Canceled) {
			report.Code = "canceled"
		}
	} else if runErr != nil && report.Status == "success" {
		report.Status = "failed"
		report.Stage = "audio_cleanup"
		report.Code = "worker_failed"
	} else if report.Status == "running" {
		report.Status = "failed"
		report.Code = "worker_failed"
	}
	return report
}
func validAudioReport(r audioReport) bool {
	if r.Status != "running" && r.Status != "failed" && r.Status != "success" {
		return false
	}
	switch r.Stage {
	case "authentication", "token_renewal", "audio_session", "audio_stream", "audio_read", "audio_cleanup", "complete", "audio_download", "ogg_verification", "mp3_conversion", "mp3_verification":
	default:
		return false
	}
	switch r.Code {
	case "", "authentication_required", "provider_changed", "disabled", "temporarily_unavailable",
		"token_request_denied", "audio_session_failed", "audio_stream_failed", "audio_key_rejected", "audio_read_failed", "ogg_header_missing", "audio_download_failed", "ogg_verification_failed", "mp3_conversion_failed", "mp3_verification_failed", "verification_limit_exceeded", "output_path_rejected":
	default:
		return false
	}
	for _, p := range []*audioPass{&r.Initial, r.AfterRenewal} {
		if p == nil {
			continue
		}
		if p.BytesRead < 0 || p.BytesRead > 16384 || (p.StreamOpened && !p.SessionAuthenticated) ||
			(p.BytesRead > 0 && !p.StreamOpened) || (p.OggHeaderVerified && p.BytesRead < 4) {
			return false
		}
	}
	if d := r.Download; d != nil {
		if r.Status == "success" && (!d.SessionAuthenticated || !d.OggDownloaded || d.Ogg == nil || !d.MP3Converted || d.MP3 == nil || !d.SourceRemoved) {
			return false
		}
		if (d.OggDownloaded && !d.SessionAuthenticated) || (d.Ogg != nil && (!d.OggDownloaded || !validPCMVerification(*d.Ogg))) || (d.MP3Converted && d.Ogg == nil) || (d.MP3 != nil && (!d.MP3Converted || !validPCMVerification(*d.MP3))) || (d.SourceRemoved && d.MP3 == nil) {
			return false
		}
	}
	return true
}
func writeAudioMilestones(path string, report audioReport) error {
	data, err := json.Marshal(report)
	if err != nil {
		return err
	}
	temporary := path + ".tmp"
	if err = os.WriteFile(temporary, data, 0600); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}
func audioWorker(ctx context.Context, id string, bytes int, renew bool, path string) error {
	// Worker files are confined to the parent's freshly-created temp directory.
	root := filepath.Clean(os.TempDir())
	dir := filepath.Dir(filepath.Clean(path))
	if filepath.Dir(dir) != root || !strings.HasPrefix(filepath.Base(dir), "viib-spotify-audio-probe-") ||
		filepath.Base(path) != "milestones.json" {
		return errAudioProbeFailed
	}
	report := audioReport{Status: "running", Stage: "authentication"}
	save := func() error { return writeAudioMilestones(path, report) }
	fail := func(code string) error {
		report.Status = "failed"
		report.Code = code
		_ = save()
		return errAudioProbeFailed
	}
	if err := save(); err != nil {
		return errAudioProbeFailed
	}
	cookie := os.Getenv("VIIB_SPOTIFY_SP_DC")
	_ = os.Unsetenv("VIIB_SPOTIFY_SP_DC")
	provider, err := auth.NewWebPlayerProvider(cookie, researchContract(), auth.WebPlayerOptions{})
	cookie = ""
	if err != nil {
		return fail(audioAuthCode(err))
	}
	defer provider.Disconnect()
	token, err := provider.Token(ctx)
	if err != nil {
		return fail(audioAuthCode(err))
	}
	report.AuthenticationVerified = true
	backend := &liveAudioBackend{directory: dir}
	defer backend.Close()
	if err = audioReadPass(ctx, backend, token.Bearer(), id, bytes, &report.Initial, &report, save); err != nil {
		return fail(audioErrorCode(err, report.Stage))
	}
	if renew {
		report.Stage = "token_renewal"
		if err = save(); err != nil {
			return errAudioProbeFailed
		}
		token, err = provider.Refresh(ctx, token)
		if err != nil {
			return fail(audioAuthCode(err))
		}
		report.RenewalVerified = true
		report.AfterRenewal = &audioPass{}
		// Always use a fresh session; equal bearer bytes must not make this pass
		// silently reuse an already authenticated session.
		backend.Close()
		backend = &liveAudioBackend{directory: dir}
		defer backend.Close()
		if err = audioReadPass(ctx, backend, token.Bearer(), id, bytes, report.AfterRenewal, &report, save); err != nil {
			return fail(audioErrorCode(err, report.Stage))
		}
	}
	report.Stage = "audio_cleanup"
	if err = save(); err != nil {
		return errAudioProbeFailed
	}
	backend.Close()
	report.Status = "success"
	report.Stage = "complete"
	return save()
}

type audioBackend interface {
	Initialize(string) error
	Open(context.Context, string) (io.ReadCloser, error)
}
type liveAudioBackend struct {
	directory string
	session   *spotify.SessionManager
	streamer  *spotify.Streamer
}

func (b *liveAudioBackend) Initialize(token string) error {
	b.session = spotify.NewSessionManager(token, filepath.Join(b.directory, "cache"))
	if err := b.session.Initialize(); err != nil {
		return err
	}
	b.streamer = spotify.NewStreamer(b.session)
	return nil
}
func (b *liveAudioBackend) Open(ctx context.Context, id string) (io.ReadCloser, error) {
	return b.streamer.StreamTrackWithQuality(ctx, id, "research-audio-probe", "low")
}
func (b *liveAudioBackend) Close() {
	if b.streamer != nil {
		b.streamer.CloseAllStreams()
		b.streamer = nil
	}
	if b.session != nil {
		_ = b.session.Close()
		b.session = nil
	}
}

var errOggHeader = errors.New("expected Ogg header absent")

func audioReadPass(ctx context.Context, backend audioBackend, bearer, id string, bytes int,
	pass *audioPass, report *audioReport, save func() error) error {
	report.Stage = "audio_session"
	if err := save(); err != nil {
		return err
	}
	if err := backend.Initialize(bearer); err != nil {
		return err
	}
	pass.SessionAuthenticated = true
	report.Stage = "audio_stream"
	if err := save(); err != nil {
		return err
	}
	stream, err := backend.Open(ctx, id)
	if err != nil {
		return err
	}
	defer stream.Close()
	pass.StreamOpened = true
	report.Stage = "audio_read"
	if err = save(); err != nil {
		return err
	}
	buffer := make([]byte, bytes)
	defer clear(buffer)
	count, err := io.ReadFull(stream, buffer)
	pass.BytesRead = count
	pass.OggHeaderVerified = count >= 4 && string(buffer[:4]) == "OggS"
	if saveErr := save(); saveErr != nil {
		return saveErr
	}
	if err != nil {
		return err
	}
	if !pass.OggHeaderVerified {
		return errOggHeader
	}
	return nil
}
func audioAuthCode(err error) string {
	switch {
	case errors.Is(err, auth.ErrAuthenticationRequired):
		return "authentication_required"
	case errors.Is(err, auth.ErrProviderChanged):
		return "provider_changed"
	case errors.Is(err, auth.ErrDisabled):
		return "disabled"
	}
	var httpError *auth.WebPlayerHTTPError
	if errors.As(err, &httpError) {
		return "token_request_denied"
	}
	return "temporarily_unavailable"
}
func audioErrorCode(err error, stage string) string {
	if errors.Is(err, errOggHeader) {
		return "ogg_header_missing"
	}
	if spotify.IsAudioKeyRejected(err) {
		return "audio_key_rejected"
	}
	switch stage {
	case "audio_session":
		return "audio_session_failed"
	case "audio_stream":
		return "audio_stream_failed"
	case "audio_read":
		return "audio_read_failed"
	}
	return "temporarily_unavailable"
}
