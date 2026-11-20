// Defines v2 jobs plex analysis functionality for package api.

package api

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/ajbergh/viib-mediahub/internal/analysis"
	"github.com/ajbergh/viib-mediahub/internal/db"
)

// resolveAnalysisSource keeps the track analyzer independent from remote
// credentials. Local songs use their normal file source; a Plex catalog row
// supplies a token-safe direct-play stream instead.
func (a *API) resolveAnalysisSource(ctx context.Context, songID string) (analysis.ResolvedSource, error) {
	plexTrack, err := a.db.GetPlexTrackSource(songID)
	if err != nil {
		return analysis.ResolvedSource{}, fmt.Errorf("load Plex analysis source: %w", err)
	}
	if plexTrack == nil {
		return analysis.ResolveLocalSource(a.db, songID)
	}
	if !plexTrack.Available || strings.TrimSpace(plexTrack.MediaKey) == "" {
		return analysis.ResolvedSource{}, fmt.Errorf("plex source unavailable")
	}
	source, err := a.db.GetPlexSource(plexTrack.SourceID)
	if err != nil || source == nil || !source.Available {
		return analysis.ResolvedSource{}, fmt.Errorf("plex source unavailable")
	}
	revision := plexAnalysisFingerprint(*plexTrack)
	name := plexAnalysisFilename(plexTrack.RatingKey, plexTrack.Container)
	return analysis.ResolvedSource{
		SongID: songID, Name: name, Path: "plex://" + plexTrack.SourceID + "/" + plexTrack.RatingKey,
		Fingerprint: revision, SourceRevision: revision,
		OpenStream: func() (io.ReadCloser, error) {
			return a.openPlexAnalysisStream(ctx, source, plexMediaInput{key: plexTrack.MediaKey, metadataKey: plexTrack.MetadataKey, audio: true})
		},
	}, nil
}

func plexAnalysisFingerprint(track db.PlexTrackSource) string {
	return strings.Join([]string{
		"plex", track.MachineID, track.RatingKey, track.MediaKey,
		strconv.FormatInt(track.UpdatedAt, 10),
	}, ":")
}

func plexAnalysisFilename(ratingKey, container string) string {
	container = strings.Trim(strings.ToLower(strings.TrimSpace(container)), ".")
	if container == "" {
		container = "audio"
	}
	return "plex-" + strings.TrimSpace(ratingKey) + "." + container
}

// openPlexAnalysisStream makes an authenticated, server-side direct-play
// request. One stream per PMS source is allowed at a time so an explicit
// library preparation run cannot saturate a remote server.
func (a *API) openPlexAnalysisStream(ctx context.Context, source *db.PlexSource, input plexMediaInput) (io.ReadCloser, error) {
	release, err := a.acquirePlexAnalysisGate(ctx, source.ID)
	if err != nil {
		return nil, err
	}
	closeOnError := true
	defer func() {
		if closeOnError {
			release()
		}
	}()
	response, err := a.openPlexMedia(ctx, source, input)
	if err != nil {
		if isPlexConnectionError(err) {
			a.recordPlexAvailability(ctx, source, false)
		}
		return nil, fmt.Errorf("plex source unavailable")
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		response.Body.Close()
		return nil, fmt.Errorf("plex source unavailable")
	}
	closeOnError = false
	return &releaseReadCloser{ReadCloser: response.Body, release: release}, nil
}

func (a *API) acquirePlexAnalysisGate(ctx context.Context, sourceID string) (func(), error) {
	a.plexAnalysisMu.Lock()
	if a.plexAnalysisGates == nil {
		a.plexAnalysisGates = make(map[string]chan struct{})
	}
	gate := a.plexAnalysisGates[sourceID]
	if gate == nil {
		gate = make(chan struct{}, 1)
		a.plexAnalysisGates[sourceID] = gate
	}
	a.plexAnalysisMu.Unlock()
	select {
	case gate <- struct{}{}:
		var once sync.Once
		return func() { once.Do(func() { <-gate }) }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

type releaseReadCloser struct {
	io.ReadCloser
	release func()
	once    sync.Once
}

func (r *releaseReadCloser) Close() error {
	err := r.ReadCloser.Close()
	r.once.Do(r.release)
	return err
}
