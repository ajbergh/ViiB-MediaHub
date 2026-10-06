package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/logger"
	"github.com/ajbergh/viib-mediahub/internal/spotify/auth"
	spotifyrefresh "github.com/ajbergh/viib-mediahub/internal/spotify/refresh"
	"github.com/ajbergh/viib-mediahub/internal/spotify/waveform"
	"github.com/go-chi/chi/v5"
	"modernc.org/sqlite"
)

var spotifyWaveformPersistenceMu sync.Mutex

func (a *API) refreshSpotifyWaveform(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "trackID")
	if !db.ValidSpotifyRecordingID(id) {
		respondError(w, 400, "Invalid Spotify recording")
		return
	}
	result, retryAfter, err := a.spotifyWaveformForRecording(r.Context(), id)
	if err != nil {
		respondError(w, 503, "Spotify waveform unavailable")
		return
	}
	if retryAfter > 0 {
		w.Header().Set("Retry-After", strconv.FormatInt(int64((retryAfter+time.Second-1)/time.Second), 10))
	}
	respondJSON(w, result)
}

// spotifyWaveformForRecording is shared by explicit refresh and preparation.
// Provider artifacts remain recording/account scoped; applying them to a song
// requires the caller's source and recording-link checks.
func (a *API) spotifyWaveformForRecording(parent context.Context, id string) (map[string]any, time.Duration, error) {
	if !db.ValidSpotifyRecordingID(id) {
		return nil, 0, errors.New("invalid Spotify recording")
	}
	runtime := a.spotifyTokens()
	return runtime.shareWaveform(parent, id, func(ctx context.Context) (map[string]any, time.Duration, error) {
		return a.fetchSpotifyWaveform(ctx, id, runtime)
	})
}

func (a *API) fetchSpotifyWaveform(parent context.Context, id string, runtime *spotifyAuthRuntime) (map[string]any, time.Duration, error) {
	ctx, cancel := runtime.requestContext(parent)
	defer cancel()
	ctx, timeoutCancel := context.WithTimeout(ctx, 25*time.Second)
	defer timeoutCancel()
	if err := runtime.ensureMetadataOwner(ctx); err != nil {
		logger.Scan("spotify_resource spotify_id=%q resource=three_band_waveform status=owner_unavailable", id)
		return nil, 0, err
	}
	release, err := spotifyrefresh.AcquireProviderSlot(ctx)
	if err != nil {
		logger.Scan("spotify_resource spotify_id=%q resource=three_band_waveform status=admission_failed", id)
		return nil, 0, err
	}
	defer release()
	var artifact *db.SpotifyAudioArtifact
	var readFence db.SpotifyMetadataFence
	var readContext string
	var writeErr error
	var failureStatus *db.SpotifyMetadataResourceStatus
	key := db.SpotifySnapshotKey{EntityType: "track", SpotifyID: id, Resource: "three_band_waveform"}
	err = runtime.withAccount(ctx, func() error {
		key.ContextKey = runtime.metadataContext
		readContext = runtime.metadataContext
		readFence = db.SpotifyMetadataFence{Epoch: runtime.metadataEpoch, ContextKey: runtime.metadataContext}
		var err error
		artifact, err = a.db.GetSpotifyAudioArtifact(id, "three_band_waveform", "spotify_three_band", key.ContextKey)
		return err
	})
	if err != nil {
		return nil, 0, err
	}
	now := time.Now().UTC()
	if artifact != nil && now.Before(artifact.ExpiresAt) {
		return map[string]any{"state": "available", "fromCache": true, "representation": "spotify_three_band"}, 0, nil
	}
	cooldown, err := a.db.GetExternalAnalysisCooldownForRuntime(readFence)
	if err != nil {
		return nil, 0, err
	}
	if cooldown.After(now) {
		return map[string]any{"state": "cooldown", "retryAt": cooldown, "lastGoodAvailable": artifact != nil}, 0, nil
	}
	var status *db.SpotifyMetadataResourceStatus
	err = runtime.withAccount(ctx, func() error { var err error; status, err = a.db.GetSpotifyMetadataResourceStatus(key); return err })
	if err != nil {
		return nil, 0, err
	}
	if status != nil && status.State != "available" && status.RetryAt.After(now) {
		return map[string]any{"state": status.State, "reason": status.Reason, "retryAt": status.RetryAt, "lastGoodAvailable": artifact != nil}, 0, nil
	}
	var cached *waveform.Waveform
	etag := ""
	if artifact != nil {
		value, err := waveform.DecodeDomain(artifact.Payload)
		if err == nil {
			value.TrackID = id
			value.ETag = artifact.ProviderETag
			etag = value.ETag
			cached = &value
		}
	}
	client := waveform.NewClient(runtime, waveform.Options{Enabled: true, AppVersion: auth.PinnedWebPlayerContract().AppVersion, Client: a.spotifyHTTPClient, BeforeRequest: runtime.checkWebAPICooldown, OnRateLimit: func(value string) time.Duration {
		_ = runtime.recordWebAPICooldown(value)
		return runtime.webAPICooldownRemaining()
	}})
	fetched, fetchErr := client.Fetch(ctx, id, etag, cached)
	now = time.Now().UTC()
	err = runtime.withAccount(ctx, func() error {
		if fetchErr != nil {
			reason := "temporarily_unavailable"
			state := "failed"
			retry := now.Add(time.Minute)
			var failure *waveform.Error
			if errors.As(fetchErr, &failure) {
				reason = string(failure.Code)
				if failure.Code == "not_found" || failure.Code == "access_denied" {
					state = "unavailable"
					retry = now.Add(time.Hour)
				}
				if failure.Code == "rate_limited" {
					state = "cooldown"
					retry = now.Add(failure.RetryAfter)
				}
			}
			failureStatus = &db.SpotifyMetadataResourceStatus{State: state, Reason: reason, CheckedAt: now, RetryAt: retry}
			return nil
		}
		writeErr = putSpotifyWaveformWithRetry(ctx, a.db, db.SpotifyMetadataFence{Epoch: runtime.metadataEpoch, ContextKey: key.ContextKey}, id, fetched, now)
		if writeErr != nil {
			if sqliteWriteBusy(writeErr) {
				if artifact != nil {
					return nil
				}
				return errors.New("waveform_artifact_write_busy")
			}
			return errors.New("waveform_artifact_write_failed")
		}
		return nil
	})
	if err != nil {
		logger.Scan("spotify_resource spotify_id=%q resource=three_band_waveform status=publication_rejected context_changed=%t storage_error=%q", id, runtime.metadataContext != readContext, safeWaveformStoreReason(writeErr, err))
		return nil, 0, err
	}
	if failureStatus != nil {
		if err := a.db.PutSpotifyMetadataResourceStatusForRuntime(readFence, key, *failureStatus); err != nil {
			logger.Scan("spotify_resource spotify_id=%q resource=three_band_waveform status=attempt_status_write_failed", id)
		}
	}
	if fetchErr != nil {
		reason := "temporarily_unavailable"
		var failure *waveform.Error
		if errors.As(fetchErr, &failure) {
			reason = string(failure.Code)
		}
		logger.Scan("spotify_resource spotify_id=%q resource=three_band_waveform status=unavailable reason=%q last_good=%t", id, reason, artifact != nil)
		var retryAfter time.Duration
		if errors.As(fetchErr, &failure) && failure.Code == "rate_limited" {
			retryAfter = failure.RetryAfter
		}
		return map[string]any{"state": "unavailable", "lastGoodAvailable": artifact != nil}, retryAfter, nil
	}
	logger.Scan("spotify_resource spotify_id=%q resource=three_band_waveform status=available samples_per_band=%d window_ms=%d", id, len(fetched.Lows), fetched.WindowMilliseconds)
	return map[string]any{"state": "available", "fromCache": false, "representation": "spotify_three_band"}, 0, nil
}

func safeWaveformStoreReason(writeErr, fenceErr error) string {
	if writeErr != nil {
		return writeErr.Error()
	}
	if fenceErr != nil {
		return "runtime_account_fence_rejected"
	}
	return "unknown"
}

func sqliteWriteBusy(err error) bool {
	var sqliteErr *sqlite.Error
	if !errors.As(err, &sqliteErr) {
		return false
	}
	code := sqliteErr.Code()
	primary := code & 0xff
	return primary == 5 || primary == 6
}

func putSpotifyWaveformWithRetry(ctx context.Context, database *db.DB, fence db.SpotifyMetadataFence, id string, value waveform.Waveform, retrieved time.Time) error {
	spotifyWaveformPersistenceMu.Lock()
	defer spotifyWaveformPersistenceMu.Unlock()
	backoff := []time.Duration{10 * time.Millisecond, 25 * time.Millisecond, 50 * time.Millisecond}
	var err error
	for attempt := 0; ; attempt++ {
		err = database.PutSpotifyWaveformForRuntime(fence, id, value, waveform.ContractRevision, retrieved, retrieved.Add(7*24*time.Hour))
		if !sqliteWriteBusy(err) || attempt == len(backoff) {
			return err
		}
		timer := time.NewTimer(backoff[attempt])
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}
