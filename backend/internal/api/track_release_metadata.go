package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/logger"
	"github.com/ajbergh/viib-mediahub/internal/releasedate"
	"github.com/go-chi/chi/v5"
)

// Spotify's track response identifies an album edition, not a recording's
// first release. Never promote this field into original_year.
func (a *API) spotifyTrackEditionDate(parent context.Context, id string) string {
	if !db.ValidSpotifyRecordingID(id) {
		return ""
	}
	ctx, cancel := context.WithTimeout(parent, 15*time.Second)
	defer cancel()
	response, err := a.doSpotifyRequest(ctx, http.MethodGet, "https://api.spotify.com/v1/tracks/"+id, nil, "")
	if err != nil || response == nil {
		return ""
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return ""
	}
	var track struct {
		ID    string `json:"id"`
		Album struct {
			Date string `json:"release_date"`
		} `json:"album"`
	}
	if json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&track) != nil || track.ID != id {
		return ""
	}
	date, _, _ := releasedate.Parse(track.Album.Date)
	return date
}

func (a *API) startReleaseMetadataWorker() {
	ctx, finish, ok := a.beginEnrichment(context.Background())
	if !ok {
		return
	}
	go func() {
		defer finish()
		timer := time.NewTimer(30 * time.Second)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
			}
			songs, err := a.db.ReleaseMetadataCandidates(ctx, 8)
			if err != nil && ctx.Err() == nil {
				logger.API("Release date backfill: %v", err)
			}
			for _, song := range songs {
				if ctx.Err() != nil {
					return
				}
				if err := a.resolveTrackReleaseMetadata(ctx, song); err != nil && ctx.Err() == nil {
					logger.API("Release date lookup failed for track %s: %v", song.ID, err)
				}
			}
			timer.Reset(30 * time.Second)
		}
	}()
}

func (a *API) resolveTrackReleaseMetadata(ctx context.Context, song db.Song) error {
	value, err := a.db.GetTrackReleaseMetadata(song.ID)
	if err != nil {
		return err
	}
	value.CheckedAt = time.Now().UnixMilli()
	// Keep estimates in storage, but do not submit them as verified dates.
	if value.OriginalSource != "musicbrainz" && value.OriginalSource != "file_tag" {
		value.OriginalDate, value.OriginalSource = "", ""
	}
	fingerprints, err := a.currentAnalysisSourceFingerprints([]string{song.ID})
	if err != nil {
		return err
	}
	fingerprint := fingerprints[song.ID]
	if fingerprint == "" {
		value.State = "source_unavailable"
		value.RetryAt = time.Now().Add(time.Hour).UnixMilli()
		_, err = a.db.ApplyTrackReleaseMetadata(ctx, song, value)
		return err
	}
	if value.EditionDate == "" {
		link, linkErr := a.db.GetSpotifyRecording(song.ID, fingerprint)
		if linkErr == nil && link != nil {
			value.EditionDate = a.spotifyTrackEditionDate(ctx, link.ExternalID)
			if value.EditionDate != "" {
				value.EditionSource = "spotify"
			}
		}
		if value.EditionDate == "" && song.Year > 0 {
			value.EditionDate = releasedate.YearDate(song.Year)
			value.EditionSource = "existing_metadata"
		}
	}
	if value.OriginalDate == "" || value.OriginalSource != "file_tag" && value.OriginalSource != "musicbrainz" {
		match, lookupErr := a.releaseMusicBrainz.Lookup(ctx, song.Title, song.Artist, song.Duration)
		value.State = match.State
		if lookupErr != nil {
			value.State = "unavailable"
			value.RetryAt = time.Now().Add(time.Hour).UnixMilli()
			var cooldown *releasedate.CooldownError
			if errors.As(lookupErr, &cooldown) {
				value.RetryAt = cooldown.Until.UnixMilli()
			}
		} else {
			value.RetryAt = time.Now().Add(24 * time.Hour).UnixMilli()
			if match.Date != "" && (releasedate.Year(value.EditionDate) == 0 || releasedate.Year(match.Date) <= releasedate.Year(value.EditionDate)) {
				value.OriginalDate, value.OriginalSource, value.RecordingID = match.Date, "musicbrainz", match.RecordingID
			} else if match.Date != "" {
				value.State = "conflicting_dates"
			}
		}
	} else {
		value.State = "available"
		value.RetryAt = time.Now().Add(30 * 24 * time.Hour).UnixMilli()
	}
	// Never reclassify an existing AI estimate as verified while recording an attempt.
	if value.OriginalSource != "musicbrainz" && value.OriginalSource != "file_tag" {
		value.OriginalDate, value.OriginalSource = "", ""
	}
	current, err := a.currentAnalysisSourceFingerprints([]string{song.ID})
	if err != nil {
		return err
	}
	if current[song.ID] != fingerprint {
		return nil
	}
	_, err = a.db.ApplyTrackReleaseMetadata(ctx, song, value)
	return err
}

func (a *API) getTrackReleaseMetadata(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "songID")
	song, err := a.db.GetSongByID(id)
	if err != nil || song == nil {
		respondError(w, http.StatusNotFound, "Track not found")
		return
	}
	value, err := a.db.GetTrackReleaseMetadata(id)
	if err != nil {
		respondError(w, 500, "Release metadata unavailable")
		return
	}
	if value.OriginalDate == "" && song.OriginalYear > 0 {
		value.OriginalDate = releasedate.YearDate(song.OriginalYear)
		value.OriginalSource = "unverified"
	}
	w.Header().Set("Cache-Control", "no-store")
	respondJSON(w, value)
}
