package track

import (
	"strconv"

	"github.com/ajbergh/viib-mediahub/internal/analysis/key"
	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/logger"
	spotifyanalysis "github.com/ajbergh/viib-mediahub/internal/spotify/analysis"
)

func scanEngineDecision(observation *spotifyanalysis.Observation, hasLookup bool) (engine, reason string) {
	if !hasLookup {
		return "local", "spotify_not_configured"
	}
	if observation == nil {
		return "local", "spotify_unavailable"
	}
	bpm, tonalKey := observation.BPM != nil, observation.Key != nil && observation.Mode != nil
	switch {
	case bpm && tonalKey:
		return "spotify+local", "spotify_scalars_local_artifacts"
	case bpm:
		return "spotify+local", "spotify_missing_key"
	case tonalKey:
		return "spotify+local", "spotify_missing_bpm"
	default:
		return "local", "spotify_missing_bpm_and_key"
	}
}

func logScanResult(result Result, localEngine string) {
	record := db.TrackAnalysis{SongID: result.SongID, Status: result.Status}
	if result.Tempo.Known {
		record.BPM = &result.Tempo.BPM
		record.BPMSource = ptr("measured")
	}
	if result.Key.Known {
		record.KeyTonic = &result.Key.Tonic
		record.KeyMode = &result.Key.Mode
		record.KeySource = ptr("measured")
	}
	if result.Spotify != nil {
		db.ApplySpotifyScalars(&record, *result.Spotify)
	}
	logScanRecord(record, "analyzed", localEngine)
}

func logScanRecord(record db.TrackAnalysis, action, localEngine string) {
	bpmSource, keySource, bpmValue, keyValue := "unknown", "unknown", "unknown", "unknown"
	if record.BPM != nil && record.BPMSource != nil {
		bpmSource = *record.BPMSource
		if bpmSource == "measured" {
			bpmSource = "local"
		}
		bpmValue = strconv.FormatFloat(*record.BPM, 'f', -1, 64)
	}
	if record.KeyTonic != nil && record.KeyMode != nil && record.KeySource != nil {
		keySource = *record.KeySource
		if keySource == "measured" {
			keySource = "local"
		}
		keyValue = key.FormatKey(*record.KeyTonic, *record.KeyMode)
	}
	logger.Scan("analysis_result song_id=%q action=%q status=%q bpm_source=%q bpm=%s key_source=%q key=%q local_engine=%q", record.SongID, action, record.Status, bpmSource, bpmValue, keySource, keyValue, localEngine)
}
