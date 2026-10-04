//go:build spotify_research

// Runs isolated download, container, decoder, and metadata probes with bounded cleanup.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	localanalysis "github.com/ajbergh/viib-mediahub/internal/analysis"
	"github.com/ajbergh/viib-mediahub/internal/audio"
	"github.com/ajbergh/viib-mediahub/internal/spotify"
	"github.com/ajbergh/viib-mediahub/internal/spotify/auth"
	mp3decoder "github.com/hajimehoshi/go-mp3"
)

const downloadProbeTimeout = 180 * time.Second
const maxProbeAudioFileBytes int64 = 128 << 20
const maxProbeAudioSeconds = 1800

var errDownloadLimit = errors.New("download probe verification limit exceeded")
var errDownloadPath = errors.New("download probe path outside isolated directory")
var errIncompleteAudio = errors.New("download probe incomplete audio")

type pcmVerification struct {
	FileBytes       int64   `json:"fileBytes"`
	SampleRate      int     `json:"sampleRate"`
	Channels        int     `json:"decodedChannels"`
	DeclaredFrames  int64   `json:"declaredFrames,omitempty"`
	DecodedFrames   int64   `json:"decodedFrames"`
	DurationSeconds float64 `json:"durationSeconds"`
}
type downloadVerification struct {
	SessionAuthenticated bool             `json:"sessionAuthenticated"`
	OggDownloaded        bool             `json:"oggDownloaded"`
	Ogg                  *pcmVerification `json:"ogg,omitempty"`
	MP3Converted         bool             `json:"mp3Converted"`
	MP3                  *pcmVerification `json:"mp3,omitempty"`
	SourceRemoved        bool             `json:"sourceRemoved"`
}

func validateDownloadOptions(enabled bool, audioBytes int, webAPI, features, compare bool) error {
	if enabled && (audioBytes != 0 || webAPI || features || compare) {
		return errors.New("choose one research mode")
	}
	return nil
}
func downloadMode(ctx context.Context, id string, renew bool, out io.Writer) error {
	if path := os.Getenv(audioWorkerReportEnv); path != "" {
		_ = os.Unsetenv(audioWorkerReportEnv)
		return downloadWorker(ctx, id, renew, path)
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
	args := []string{"--enable", "--track-id", id, "--download", "--renew=" + strconv.FormatBool(renew)}
	environment := append(os.Environ(), audioWorkerReportEnv+"="+path)
	_ = os.Unsetenv("VIIB_SPOTIFY_SP_DC")
	report := executeAudioWorker(ctx, executable, args, environment, path, downloadProbeTimeout)
	if err = json.NewEncoder(out).Encode(report); err != nil {
		return err
	}
	if report.Status != "success" {
		return errAudioProbeFailed
	}
	return nil
}
func isolatedWorkerDirectory(path string) (string, bool) {
	root := filepath.Clean(os.TempDir())
	directory := filepath.Dir(filepath.Clean(path))
	return directory, filepath.Dir(directory) == root && strings.HasPrefix(filepath.Base(directory), "viib-spotify-audio-probe-") && filepath.Base(path) == "milestones.json"
}
func downloadWorker(ctx context.Context, id string, renew bool, path string) error {
	directory, valid := isolatedWorkerDirectory(path)
	if !valid {
		return errAudioProbeFailed
	}
	report := audioReport{Status: "running", Stage: "authentication", Download: &downloadVerification{}}
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
	}
	session := spotify.NewSessionManager(token.Bearer(), filepath.Join(directory, "cache"))
	defer session.Close()
	root := filepath.Join(directory, "download")
	ops := downloadOperations{
		Initialize: session.Initialize,
		Download: func(ctx context.Context, id string) (string, error) {
			bounded, cancel := context.WithCancel(ctx)
			defer cancel()
			limited := false
			downloader := spotify.NewDownloader(session, root)
			path, err := downloader.DownloadTrack(bounded, id, "probe-artist", "probe-track", "probe-album", nil,
				func(received, total int64) {
					if received > maxProbeAudioFileBytes {
						limited = true
						cancel()
					}
				}, nil)
			if limited {
				return "", errDownloadLimit
			}
			return path, err
		},
		VerifyOgg: verifyProbeOgg,
		Convert: func(ctx context.Context, path string) (string, error) {
			return audio.ConvertOggToMP3(ctx, path, audio.MP3Metadata{Title: "probe-track", Artist: "probe-artist", Album: "probe-album"}, nil)
		},
		VerifyMP3: verifyProbeMP3,
	}
	if err = runDownloadPass(ctx, ops, id, root, &report, save); err != nil {
		return fail(downloadErrorCode(err, report.Stage))
	}
	report.Stage = "audio_cleanup"
	if err = save(); err != nil {
		return errAudioProbeFailed
	}
	_ = session.Close()
	report.Status = "success"
	report.Stage = "complete"
	return save()
}

type downloadOperations struct {
	Initialize func() error
	Download   func(context.Context, string) (string, error)
	VerifyOgg  func(context.Context, string) (pcmVerification, error)
	Convert    func(context.Context, string) (string, error)
	VerifyMP3  func(context.Context, string) (pcmVerification, error)
}

func runDownloadPass(ctx context.Context, ops downloadOperations, id, root string, report *audioReport, save func() error) error {
	metrics := report.Download
	if metrics == nil {
		return errIncompleteAudio
	}
	report.Stage = "audio_session"
	if err := save(); err != nil {
		return err
	}
	if err := ops.Initialize(); err != nil {
		return err
	}
	metrics.SessionAuthenticated = true
	report.Stage = "audio_download"
	if err := save(); err != nil {
		return err
	}
	oggPath, err := ops.Download(ctx, id)
	if err != nil {
		return err
	}
	if err = confinedProbeFile(root, oggPath, ".ogg"); err != nil {
		return err
	}
	metrics.OggDownloaded = true
	report.Stage = "ogg_verification"
	if err = save(); err != nil {
		return err
	}
	ogg, err := ops.VerifyOgg(ctx, oggPath)
	if err != nil {
		return err
	}
	if !validPCMVerification(ogg) {
		return errIncompleteAudio
	}
	metrics.Ogg = &ogg
	report.Stage = "mp3_conversion"
	if err = save(); err != nil {
		return err
	}
	mp3Path, err := ops.Convert(ctx, oggPath)
	if err != nil {
		return err
	}
	if err = confinedProbeFile(root, mp3Path, ".mp3"); err != nil {
		return err
	}
	metrics.MP3Converted = true
	report.Stage = "mp3_verification"
	if err = save(); err != nil {
		return err
	}
	mp3, err := ops.VerifyMP3(ctx, mp3Path)
	if err != nil {
		return err
	}
	if !validPCMVerification(mp3) || mp3.SampleRate != ogg.SampleRate || math.Abs(mp3.DurationSeconds-ogg.DurationSeconds) > 1 {
		return errIncompleteAudio
	}
	metrics.MP3 = &mp3
	_, err = os.Stat(oggPath)
	metrics.SourceRemoved = errors.Is(err, os.ErrNotExist)
	if !metrics.SourceRemoved {
		return errIncompleteAudio
	}
	return save()
}
func confinedProbeFile(root, path, extension string) error {
	if !strings.EqualFold(filepath.Ext(path), extension) {
		return errDownloadPath
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(resolvedRoot, resolved)
	if err != nil || filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return errDownloadPath
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 {
		return errIncompleteAudio
	}
	if info.Size() > maxProbeAudioFileBytes {
		return errDownloadLimit
	}
	return nil
}
func validPCMVerification(v pcmVerification) bool {
	if v.FileBytes <= 0 || v.FileBytes > maxProbeAudioFileBytes || v.SampleRate < 8000 || v.SampleRate > 192000 ||
		(v.Channels != 1 && v.Channels != 2) || v.DecodedFrames <= 0 || v.DeclaredFrames < 0 ||
		math.IsNaN(v.DurationSeconds) || math.IsInf(v.DurationSeconds, 0) || v.DurationSeconds <= 0 || v.DurationSeconds > maxProbeAudioSeconds {
		return false
	}
	if v.DeclaredFrames > 0 && v.DeclaredFrames != v.DecodedFrames {
		return false
	}
	return math.Abs(v.DurationSeconds-float64(v.DecodedFrames)/float64(v.SampleRate)) < 0.000001
}
func probeFileBytes(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 {
		return 0, errIncompleteAudio
	}
	if info.Size() > maxProbeAudioFileBytes {
		return 0, errDownloadLimit
	}
	return info.Size(), nil
}
func verifyProbeOgg(ctx context.Context, path string) (pcmVerification, error) {
	if err := ctx.Err(); err != nil {
		return pcmVerification{}, err
	}
	size, err := probeFileBytes(path)
	if err != nil {
		return pcmVerification{}, err
	}
	file, err := os.Open(path)
	if err != nil {
		return pcmVerification{}, err
	}
	stream, err := (localanalysis.VorbisDecoder{}).Open(ctx, file)
	if err != nil {
		file.Close()
		return pcmVerification{}, err
	}
	defer stream.Close()
	info := stream.Info()
	if (info.Channels != 1 && info.Channels != 2) || info.SampleRate < 8000 || info.SampleRate > 192000 ||
		info.DeclaredFrames > int64(info.SampleRate)*maxProbeAudioSeconds {
		return pcmVerification{}, errDownloadLimit
	}
	buffer := make([]float32, 8192*info.Channels)
	defer clear(buffer)
	var values int64
	for {
		count, readErr := stream.Read(ctx, buffer)
		values += int64(count)
		if values > int64(info.SampleRate)*int64(info.Channels)*maxProbeAudioSeconds {
			return pcmVerification{}, errDownloadLimit
		}
		for _, value := range buffer[:count] {
			if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
				return pcmVerification{}, errIncompleteAudio
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return pcmVerification{}, readErr
		}
		if count == 0 {
			return pcmVerification{}, io.ErrNoProgress
		}
	}
	if values%int64(info.Channels) != 0 {
		return pcmVerification{}, errIncompleteAudio
	}
	frames := values / int64(info.Channels)
	result := pcmVerification{FileBytes: size, SampleRate: info.SampleRate, Channels: info.Channels, DeclaredFrames: info.DeclaredFrames, DecodedFrames: frames, DurationSeconds: float64(frames) / float64(info.SampleRate)}
	if !validPCMVerification(result) {
		return pcmVerification{}, errIncompleteAudio
	}
	return result, nil
}
func verifyProbeMP3(ctx context.Context, path string) (pcmVerification, error) {
	if err := ctx.Err(); err != nil {
		return pcmVerification{}, err
	}
	size, err := probeFileBytes(path)
	if err != nil {
		return pcmVerification{}, err
	}
	file, err := os.Open(path)
	if err != nil {
		return pcmVerification{}, err
	}
	defer file.Close()
	decoder, err := mp3decoder.NewDecoder(file)
	if err != nil {
		return pcmVerification{}, err
	}
	rate := decoder.SampleRate()
	if rate < 8000 || rate > 192000 {
		return pcmVerification{}, errIncompleteAudio
	}
	// go-mp3 always emits interleaved stereo 16-bit PCM, including mono sources.
	buffer := make([]byte, 65536)
	defer clear(buffer)
	var pcmBytes int64
	for {
		if err = ctx.Err(); err != nil {
			return pcmVerification{}, err
		}
		count, readErr := decoder.Read(buffer)
		pcmBytes += int64(count)
		if pcmBytes > int64(rate)*4*maxProbeAudioSeconds {
			return pcmVerification{}, errDownloadLimit
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return pcmVerification{}, readErr
		}
		if count == 0 {
			return pcmVerification{}, io.ErrNoProgress
		}
	}
	if pcmBytes%4 != 0 {
		return pcmVerification{}, errIncompleteAudio
	}
	frames := pcmBytes / 4
	result := pcmVerification{FileBytes: size, SampleRate: rate, Channels: 2, DecodedFrames: frames, DurationSeconds: float64(frames) / float64(rate)}
	if !validPCMVerification(result) {
		return pcmVerification{}, errIncompleteAudio
	}
	return result, nil
}
func downloadErrorCode(err error, stage string) string {
	if errors.Is(err, errDownloadLimit) {
		return "verification_limit_exceeded"
	}
	if errors.Is(err, errDownloadPath) {
		return "output_path_rejected"
	}
	if spotify.IsAudioKeyRejected(err) {
		return "audio_key_rejected"
	}
	switch stage {
	case "audio_session":
		return "audio_session_failed"
	case "audio_download":
		return "audio_download_failed"
	case "ogg_verification":
		return "ogg_verification_failed"
	case "mp3_conversion":
		return "mp3_conversion_failed"
	case "mp3_verification":
		return "mp3_verification_failed"
	}
	return "temporarily_unavailable"
}
