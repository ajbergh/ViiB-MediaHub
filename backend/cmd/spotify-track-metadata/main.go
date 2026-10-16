//go:build spotify_research

// spotify-track-metadata fetches fresh scalar metadata with ViiB's production adapter.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/spotify/analysis"
	"github.com/ajbergh/viib-mediahub/internal/spotify/auth"
)

func run(ctx context.Context, args []string, out, stderr io.Writer, cookie string) error {
	flags := flag.NewFlagSet("spotify-track-metadata", flag.ContinueOnError)
	flags.SetOutput(stderr)
	backendURL := flags.String("backend-url", "", "Running ViiB loopback URL; uses its session and normal reference cache")
	playlistID := flags.String("playlist-id", "", "Compare native Web Player playlist attributes for this playlist ID (cookie mode)")
	v2 := flags.Bool("v2", false, "Fetch direct Web Player audio attributes V2")
	flags.Usage = func() {
		fmt.Fprintln(stderr, "Usage: spotify-track-metadata [--backend-url http://127.0.0.1:PORT] <Spotify track ID, URI, or URL>")
		fmt.Fprintln(stderr, "Fetches fresh audio_features using ViiB's Web Player adapter. Set VIIB_SPOTIFY_SP_DC to your session cookie.")
		fmt.Fprintln(stderr, "Use --v2 for native Web Player BPM/key; --playlist-id compares its playlist projection.")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		flags.Usage()
		return errors.New("exactly one Spotify track is required")
	}
	id, err := parseTrackInput(flags.Arg(0))
	if err != nil {
		return err
	}
	if *backendURL != "" {
		if *playlistID != "" {
			return errors.New("playlist comparison requires cookie mode; omit --backend-url")
		}
		endpoint := "audio_features"
		if *v2 {
			endpoint = analysis.AttributesV2
		}
		return fetchFromBackend(ctx, *backendURL, id, out, endpoint)
	}
	if *v2 && *playlistID != "" {
		return errors.New("choose direct --v2 or --playlist-id comparison")
	}
	if *playlistID != "" {
		if _, err := parseTrackInput(*playlistID); err != nil {
			return errors.New("playlist-id must be a 22-character Spotify ID")
		}
	}
	if cookie == "" {
		return errors.New("set VIIB_SPOTIFY_SP_DC to your Spotify sp_dc session cookie")
	}
	contract := auth.PinnedWebPlayerContract()
	provider, err := auth.NewWebPlayerProvider(cookie, contract, auth.WebPlayerOptions{})
	cookie = ""
	if err != nil {
		return fmt.Errorf("Spotify session: %w", err)
	}
	defer provider.Disconnect()
	manager := auth.NewWebPlayerManager(provider)
	client := analysis.NewClient(manager, analysis.Options{Enabled: true, AppVersion: contract.AppVersion})
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	if *playlistID != "" {
		return writePlaylistMetadata(ctx, manager, contract, *playlistID, id, out)
	}
	if *v2 {
		return writeMetadata(ctx, id, contract, client.FetchAudioAttributesV2, out)
	}
	return writeMetadata(ctx, id, contract, client.FetchFeatures, out)
}

func writeMetadata(ctx context.Context, id string, contract auth.WebPlayerContract, fetch func(context.Context, string) (analysis.Observation, error), out io.Writer) error {
	observation, err := fetch(ctx, id)
	if err != nil {
		var upstream *analysis.Error
		if errors.As(err, &upstream) {
			return fmt.Errorf("Spotify metadata: %s (HTTP %d, retry after %s)", upstream.Code, upstream.HTTPStatus, upstream.RetryAfter)
		}
		return fmt.Errorf("Spotify metadata: %w", err)
	}
	encoder := json.NewEncoder(out)
	revision := contract.Revision
	if observation.SourceEndpoint == analysis.AttributesV2 {
		revision += ";" + analysis.AttributesV2Revision
	}
	encoder.SetIndent("", "  ")
	return encoder.Encode(struct {
		AdapterRevision string               `json:"adapterRevision"`
		AppVersion      string               `json:"appVersion"`
		CacheBypassed   bool                 `json:"cacheBypassed"`
		Observation     analysis.Observation `json:"observation"`
	}{revision, contract.AppVersion, true, observation})
}

func main() {
	cookie := os.Getenv("VIIB_SPOTIFY_SP_DC")
	_ = os.Unsetenv("VIIB_SPOTIFY_SP_DC")
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	if err := run(ctx, os.Args[1:], os.Stdout, os.Stderr, cookie); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
