package api

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/analysis"
	"github.com/ajbergh/viib-mediahub/internal/analysis/threeband"
	"github.com/ajbergh/viib-mediahub/internal/analysis/track"
	"github.com/ajbergh/viib-mediahub/internal/analysisbench"
	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/logger"
	spotifyanalysis "github.com/ajbergh/viib-mediahub/internal/spotify/analysis"
	spotifyrefresh "github.com/ajbergh/viib-mediahub/internal/spotify/refresh"
	"github.com/ajbergh/viib-mediahub/internal/spotify/waveform"
)

func TestPreparationUsesLastGoodSpotifyWaveformAfterRefreshFailure(t *testing.T) {
	a, _, _ := fixtureCookieRuntime(t)
	runtime := a.spotifyTokens()

	mediaPath := filepath.Join(t.TempDir(), "song.wav")
	fixture, err := analysisbench.NewClickTrack("clicks", 128, 4, 22050, 1)
	if err != nil {
		t.Fatal(err)
	}
	var audio bytes.Buffer
	if err := analysisbench.WriteWAVPCM16(&audio, fixture); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mediaPath, audio.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	if err := a.db.SaveSong(&db.Song{ID: "waveform-song", Title: "Song", Artist: "Artist", Album: "Album", FilePath: mediaPath, AddedAt: 1}); err != nil {
		t.Fatal(err)
	}
	source, err := analysis.ResolveLocalSource(a.db, "waveform-song")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.db.RefreshTrackAnalysisSourceRevision("waveform-song", source.Fingerprint); err != nil {
		t.Fatal(err)
	}
	if _, err := a.db.ConfirmSpotifyRecording("waveform-song", cachedSpotifyID, source.Fingerprint, true); err != nil {
		t.Fatal(err)
	}
	cached, err := waveform.DecodeResponse(waveformWireFixture(200), cachedSpotifyID)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()

	a.spotifyHTTPClient = &http.Client{Transport: sessionTransport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(waveformWireFixture(404))))}, nil
	})}
	provider := &spotifyRefreshFixtureProvider{fn: func(context.Context, string, string) (spotifyanalysis.Observation, error) {
		return spotifyanalysis.Observation{}, &spotifyanalysis.Error{Code: spotifyanalysis.NotFound}
	}}
	service, err := spotifyrefresh.New(a.db, provider, spotifyrefresh.Options{Enabled: true, AdapterRevision: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	if err := a.InstallSpotifyAnalysisService(service); err != nil {
		t.Fatal(err)
	}
	if err := runtime.connect(t.Context(), "fixture-cookie"); err != nil {
		t.Fatal(err)
	}
	if err := a.db.PutSpotifyWaveform(cachedSpotifyID, runtime.metadataContext, cached, waveform.ContractRevision, now.Add(-2*time.Hour), now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := logger.Init(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	defer logger.Close()

	observation := a.spotifyPreparationForSource(t.Context(), source)
	if observation == nil || !observation.ProviderThreeBandAvailable {
		t.Fatalf("expired but valid Spotify waveform was not selected after refresh failure: %+v", observation)
	}
	progress, err := track.Run(t.Context(), a.db, analysis.NewDefaultDecoderRegistry(), []string{"waveform-song"}, track.RunOptions{ProviderPreparation: a.spotifyPreparationForSource})
	if err != nil || progress.Processed != 1 {
		t.Fatalf("preparation run = %+v, %v", progress, err)
	}
	if _, err := a.db.GetTrackAnalysisArtifact("waveform-song", threeband.Kind, threeband.FormatVersion, threeband.AlgorithmVersion); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("stale-but-validated Spotify waveform produced a duplicate local waveform: %v", err)
	}
	record, err := a.db.GetTrackAnalysis("waveform-song")
	if err != nil {
		t.Fatal(err)
	}
	states, err := a.db.GetTrackCapabilityStatuses("waveform-song", record.SourceFingerprint)
	if err != nil || states[threeband.Kind].State != "available" || states[threeband.Kind].Reason != "provider_three_band" {
		t.Fatalf("last-good provider waveform not retained as the single representation: %+v %v", states[threeband.Kind], err)
	}
	if _, err := a.db.GetSpotifyAudioArtifact(cachedSpotifyID, "three_band_waveform", "spotify_three_band", runtime.metadataContext); err != nil {
		t.Fatalf("provider last-good waveform was not preserved: %v", err)
	}
}
