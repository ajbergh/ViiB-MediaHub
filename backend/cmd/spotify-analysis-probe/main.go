//go:build spotify_research

// This diagnostic is intentionally absent from normal application builds.
// Protocol evidence: stephancill/stupid-social at f0f8c219e43d394c84516a3bcc7af7c9fd41f713,
// scripts/spotify-web-client.py (Apache-2.0). No upstream executable code is loaded.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/ajbergh/viib-mediahub/internal/spotify/analysis"
	"github.com/ajbergh/viib-mediahub/internal/spotify/auth"
	"os"
	"os/signal"
	"time"
)

const contractRevision = "stupid-social:f0f8c219e43d394c84516a3bcc7af7c9fd41f713"

func researchContract() auth.WebPlayerContract { return auth.PinnedWebPlayerContract() }
func run(ctx context.Context) error {
	enabled := flag.Bool("enable", false, "Explicitly enable the private Spotify research probe")
	id := flag.String("track-id", "", "Spotify song link, track URI, or bare track ID")
	renew := flag.Bool("renew", true, "Also test token renewal before analysis")
	compare := flag.Bool("compare-features", false, "Compare fixed audio-analysis and audio-features routes (research only)")
	features := flag.Bool("features", false, "Request scalar audio features instead of detailed audio analysis")
	audioBytes := flag.Int("stream-read-bytes", 0, "Research-only short audio read (4096..16384 bytes); disabled by default")
	verifyAppDownloads := flag.String("verify-app-downloads", "", "Research-only read-only full decode of isolated production app audio files")
	catalogTrack := flag.Bool("catalog-track", false, "Research-only fixed individual and batch track normalization")
	historyEvents := flag.Bool("history-events", false, "Research-only fixed experimental listening-history read; no production parity claim")
	historyContexts := flag.Bool("history-contexts", false, "Research-only fixed internal recent-context pages; does not assert track-event parity")
	catalogLibraryFull := flag.Bool("catalog-library-full", false, "Research-only bounded full saved-library traversal")
	catalogLibrary := flag.Bool("catalog-library", false, "Research-only fixed saved-library pages using the production adapter")
	savedPlaylist := flag.Bool("catalog-saved-playlist", false, "Research-only bounded saved-playlist full-page shape and normalization")
	catalogPlaylist := flag.Bool("catalog-playlist", false, "Research-only fixed playlist and content-page normalization")
	catalogArtist := flag.Bool("catalog-artist", false, "Research-only fixed artist and top-track normalization")
	catalogAlbum := flag.Bool("catalog-album", false, "Research-only fixed album and track-page normalization")
	catalogSearch := flag.Bool("catalog-search", false, "Research-only fixed two-page verification of the production search adapter")
	catalogProfile := flag.Bool("catalog-profile", false, "Research-only redacted verification of the production profile adapter")
	pathfinder := flag.Bool("pathfinder", false, "Research-only fixed Web Player profile and top-results reads")
	webAPI := flag.Bool("web-api", false, "Probe fixed read-only public API routes with the WebPlayer token")
	download := flag.Bool("download", false, "Research-only single-track temporary download, full decode and MP3 conversion")
	surface := flag.String("web-api-surface", "all", "Fixed API group: all, profile, track, search, library, catalog, recent (requires --web-api)")
	reconnect := flag.Bool("reconnect", false, "Research-only provider disconnect/reconnect before the selected Web API group")
	flag.Parse()
	modeCount := 0
	for _, selected := range []bool{*pathfinder, *catalogProfile, *catalogSearch, *catalogAlbum, *catalogArtist, *catalogPlaylist, *savedPlaylist, *catalogLibrary, *catalogLibraryFull, *historyContexts, *historyEvents, *catalogTrack, *verifyAppDownloads != ""} {
		if selected {
			modeCount++
		}
	}
	if modeCount > 1 || (modeCount > 0 && (*webAPI || *download || *audioBytes != 0 || *features || *compare || *reconnect || *surface != "all")) {
		return fmt.Errorf("catalog modes cannot be combined with other research modes")
	}
	if !validWebAPISurface(*surface) || (!*webAPI && (*surface != "all" || *reconnect)) {
		return errWebAPIModes
	}
	if !*enabled {
		return fmt.Errorf("research probe disabled; explicit --enable required")
	}
	// Offline artifact verification has no recording or authentication input.
	if *verifyAppDownloads != "" {
		return verifyAppDownloadArtifacts(ctx, *verifyAppDownloads, os.Stdout)
	}
	trackID, err := parseTrackInput(*id)
	if err != nil {
		return &probeFailure{stage: "track_input", cause: err}
	}
	if err := validateAudioOptions(*audioBytes, *compare, *features); err != nil {
		return err
	}
	if err := validateWebAPIOptions(*webAPI, *audioBytes, *features, *compare); err != nil {
		return err
	}
	if err := validateDownloadOptions(*download, *audioBytes, *webAPI, *features, *compare); err != nil {
		return err
	}
	contract := researchContract()
	code, err := auth.TOTP(contract.Secret, time.Unix(1777993436, 0))
	if err != nil || code != "031750" {
		return fmt.Errorf("pinned protocol self-test failed")
	}
	if *download {
		return downloadMode(ctx, trackID, *renew, os.Stdout)
	}
	if *audioBytes != 0 {
		return audioMode(ctx, trackID, *audioBytes, *renew, os.Stdout)
	}
	// Session material is never accepted as a command-line argument or saved.
	cookie := os.Getenv("VIIB_SPOTIFY_SP_DC")
	_ = os.Unsetenv("VIIB_SPOTIFY_SP_DC")
	if *catalogTrack {
		defer func() { cookie = "" }()
		return runCatalogTrackResearch(ctx, cookie, contract, *renew, auth.WebPlayerOptions{}, nil, os.Stdout)
	}
	if *historyEvents {
		defer func() { cookie = "" }()
		return runHistoryResearch(ctx, cookie, contract, *renew, auth.WebPlayerOptions{}, nil, os.Stdout, true)
	}
	if *historyContexts {
		defer func() { cookie = "" }()
		return runHistoryContextResearch(ctx, cookie, contract, *renew, auth.WebPlayerOptions{}, nil, os.Stdout)
	}
	if *catalogLibraryFull {
		defer func() { cookie = "" }()
		return runCatalogLibraryTraversal(ctx, cookie, contract, *renew, auth.WebPlayerOptions{}, nil, os.Stdout, true)
	}
	if *catalogLibrary {
		defer func() { cookie = "" }()
		return runCatalogLibraryResearch(ctx, cookie, contract, *renew, auth.WebPlayerOptions{}, nil, os.Stdout)
	}
	if *savedPlaylist {
		defer func() { cookie = "" }()
		return runPlaylistResearch(ctx, cookie, contract, *renew, auth.WebPlayerOptions{}, nil, os.Stdout, true)
	}
	if *catalogPlaylist {
		defer func() { cookie = "" }()
		return runCatalogPlaylistResearch(ctx, cookie, contract, *renew, auth.WebPlayerOptions{}, nil, os.Stdout)
	}
	if *catalogArtist {
		defer func() { cookie = "" }()
		return runCatalogArtistResearch(ctx, cookie, contract, *renew, auth.WebPlayerOptions{}, nil, os.Stdout)
	}
	if *catalogAlbum {
		defer func() { cookie = "" }()
		return runCatalogAlbumResearch(ctx, cookie, contract, *renew, auth.WebPlayerOptions{}, nil, os.Stdout)
	}
	if *catalogSearch {
		defer func() { cookie = "" }()
		return runCatalogSearchResearch(ctx, cookie, contract, *renew, auth.WebPlayerOptions{}, nil, os.Stdout)
	}
	if *catalogProfile {
		defer func() { cookie = "" }()
		return runCatalogProfileResearch(ctx, cookie, contract, *renew, auth.WebPlayerOptions{}, nil, os.Stdout)
	}
	if *pathfinder {
		defer func() { cookie = "" }()
		return runPathfinderResearch(ctx, cookie, contract, *renew, auth.WebPlayerOptions{}, nil, os.Stdout)
	}
	if *webAPI {
		defer func() { cookie = "" }()
		return runWebAPIResearch(ctx, cookie, contract, trackID, *surface, *renew, *reconnect, auth.WebPlayerOptions{}, nil, os.Stdout)
	}
	provider, err := auth.NewWebPlayerProvider(cookie, contract, auth.WebPlayerOptions{})
	cookie = ""
	if err != nil {
		return err
	}
	defer provider.Disconnect()
	manager := auth.NewManager(nil, provider)
	client := analysis.NewClient(manager, analysis.Options{Enabled: true, AppVersion: contract.AppVersion})
	token, err := manager.Token(ctx, auth.InternalAnalysis)
	if err != nil {
		return &probeFailure{stage: "authentication", cause: err}
	}
	if *renew {
		if token, err = manager.Refresh(ctx, auth.InternalAnalysis, token); err != nil {
			return &probeFailure{stage: "token_renewal", authenticated: true, cause: err}
		}
	}
	if *compare {
		return compareAvailability(ctx, token, trackID, contract, *renew, os.Stdout)
	}
	var observation analysis.Observation
	stage := "audio_analysis"
	if *features {
		stage = "audio_features"
		observation, err = client.FetchFeatures(ctx, trackID)
	} else {
		observation, err = client.Fetch(ctx, trackID)
	}
	if err != nil {
		return &probeFailure{stage: stage, authenticated: true, renewed: *renew, cause: err}
	}
	// Only normalized metadata and explicit test outcomes leave the backend.
	return json.NewEncoder(os.Stdout).Encode(struct {
		Contract        string               `json:"contract"`
		RenewalVerified bool                 `json:"renewalVerified"`
		Observation     analysis.Observation `json:"observation"`
	}{contractRevision, *renew, observation})
}
func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	if err := run(ctx); err != nil {
		if !errors.Is(err, errAudioProbeFailed) {
			_ = writeFailure(os.Stderr, err)
		}
		os.Exit(1)
	}
}
