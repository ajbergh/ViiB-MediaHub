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
		return analysis.ResolvedSource{}, fmt.Errorf("Plex source unavailable")
	}
	source, err := a.db.GetPlexSource(plexTrack.SourceID)
	if err != nil || source == nil || !source.Available {
		return analysis.ResolvedSource{}, fmt.Errorf("Plex source unavailable")
	}
	revision := strings.Join([]string{
		"plex", plexTrack.MachineID, plexTrack.RatingKey, plexTrack.MediaKey,
		strconv.FormatInt(plexTrack.UpdatedAt, 10),
	}, ":")
	name := plexAnalysisFilename(plexTrack.RatingKey, plexTrack.Container)
	return analysis.ResolvedSource{
		SongID: songID, Name: name, Path: "plex://" + plexTrack.SourceID + "/" + plexTrack.RatingKey,
		Fingerprint: revision, SourceRevision: revision,
		OpenStream: func() (io.ReadCloser, error) {
			return a.openPlexAnalysisStream(ctx, source, plexTrack.MediaKey, plexTrack.MetadataKey)
		},
	}, nil
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
func (a *API) openPlexAnalysisStream(ctx context.Context, source *db.PlexSource, mediaKey, metadataKey string) (io.ReadCloser, error) {
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
	client, err := a.plexClientForSource(ctx, source)
	if err != nil {
		return nil, fmt.Errorf("Plex source unavailable")
	}
	request, err := client.MediaRequest(ctx, mediaKey)
	if err != nil {
		return nil, fmt.Errorf("Plex source unavailable")
	}
	request.Header.Set("Accept", "*/*")
	client.ApplyPlaybackIdentity(request)
	response, err := client.MediaHTTPClient().Do(request)
	if err != nil {
		_ = a.db.SetPlexSyncState(source.ID, source.LastSyncStatus, "Plex server unreachable during analysis", false, 0)
		return nil, fmt.Errorf("Plex source unavailable")
	}
	if response.StatusCode == http.StatusServiceUnavailable && strings.TrimSpace(metadataKey) != "" {
		response.Body.Close()
		if err := client.PrepareDirectPlay(ctx, metadataKey); err != nil {
			return nil, fmt.Errorf("Plex source unavailable")
		}
		request, err = client.MediaRequest(ctx, mediaKey)
		if err != nil {
			return nil, fmt.Errorf("Plex source unavailable")
		}
		request.Header.Set("Accept", "*/*")
		client.ApplyPlaybackIdentity(request)
		response, err = client.MediaHTTPClient().Do(request)
		if err != nil {
			_ = a.db.SetPlexSyncState(source.ID, source.LastSyncStatus, "Plex server unreachable during analysis", false, 0)
			return nil, fmt.Errorf("Plex source unavailable")
		}
	}
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden || response.StatusCode == 498 {
		response.Body.Close()
		_ = a.db.SetPlexSyncState(source.ID, "auth_required", "Plex authentication expired", false, 0)
		return nil, fmt.Errorf("Plex source unavailable")
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		response.Body.Close()
		return nil, fmt.Errorf("Plex source unavailable")
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
