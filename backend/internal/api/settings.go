// Defines settings functionality for package api.

package api

import (
	"encoding/json"
	"fmt"
	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/logger"
	"github.com/ajbergh/viib-mediahub/internal/validation"
	"github.com/go-chi/chi/v5"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
)

func (a *API) getSetting(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	if key == "" {
		respondError(w, http.StatusBadRequest, "Setting key is required")
		return
	}

	if !validation.IsValidSettingKey(key) {
		respondError(w, http.StatusBadRequest, "Invalid setting key")
		return
	}
	if key == SettingAutoCueMode {
		value, err := a.db.GetSetting(key)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "Failed to get setting")
			return
		}
		mode := db.NormalizeAutomaticCuePointMode(value)
		respondJSON(w, map[string]string{"key": key, "value": string(mode)})
		return
	}

	if validation.IsSensitiveSettingKey(key) {
		value, err := a.db.GetSetting(key)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "Failed to get setting")
			return
		}
		respondJSON(w, map[string]interface{}{"key": key, "value": "", "configured": value != ""})
		return
	}

	value, err := a.db.GetSetting(key)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to get setting")
		return
	}

	respondJSON(w, map[string]string{"key": key, "value": value})
}

func (a *API) setSetting(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	if key == "" {
		respondError(w, http.StatusBadRequest, "Setting key is required")
		return
	}

	if !validation.IsValidSettingKey(key) {
		respondError(w, http.StatusBadRequest, "Invalid setting key")
		return
	}

	var body struct {
		Value string `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if key == SettingAutoCueMode {
		mode, valid := db.ParseAutomaticCuePointMode(body.Value)
		if !valid {
			respondError(w, http.StatusBadRequest, "analysis_auto_cue_mode must be off, suggest, fill-empty, or replace-generated")
			return
		}
		body.Value = string(mode)
	}

	// Special handling for concurrent_downloads - validate and update download manager
	if key == "concurrent_downloads" {
		n, err := strconv.Atoi(body.Value)
		if err != nil {
			respondError(w, http.StatusBadRequest, "Invalid value for concurrent_downloads")
			return
		}
		if n < MinConcurrentDownloads || n > MaxConcurrentDownloads {
			respondError(w, http.StatusBadRequest, fmt.Sprintf("concurrent_downloads must be between %d and %d", MinConcurrentDownloads, MaxConcurrentDownloads))
			return
		}
		if a.downloadManager != nil {
			if err := a.downloadManager.SetMaxConcurrent(n); err != nil {
				respondError(w, http.StatusInternalServerError, err.Error())
				return
			}
		}
	}

	// Conversion workers are independent from download slots and may be changed
	// while conversions are queued or running.
	if key == "spotify_conversion_workers" {
		n, err := strconv.Atoi(body.Value)
		if err != nil {
			respondError(w, http.StatusBadRequest, "Invalid value for spotify_conversion_workers")
			return
		}
		if n < MinConversionWorkers || n > MaxConversionWorkers {
			respondError(w, http.StatusBadRequest, fmt.Sprintf("spotify_conversion_workers must be between %d and %d", MinConversionWorkers, MaxConversionWorkers))
			return
		}
		if a.downloadManager != nil {
			if err := a.downloadManager.SetMaxConversionWorkers(n); err != nil {
				respondError(w, http.StatusInternalServerError, err.Error())
				return
			}
		}
	}

	if key == "spotify_download_rescan_threshold" {
		threshold, err := strconv.Atoi(body.Value)
		if err != nil || threshold < 0 {
			respondError(w, http.StatusBadRequest, "spotify_download_rescan_threshold must be a non-negative integer")
			return
		}
		if a.scanner != nil {
			a.scanner.SetRescanThreshold(threshold)
		}
	}

	// Special handling for spotify_download_path - update download manager and scanner
	if key == "spotify_download_path" {
		downloadDir := body.Value
		if downloadDir == "" {
			// Use default if empty
			downloadDir = filepath.Join(a.dataDir, "spotify_downloads")
		}
		// Create the directory if it doesn't exist
		if err := os.MkdirAll(downloadDir, 0700); err != nil {
			respondError(w, http.StatusBadRequest, fmt.Sprintf("Failed to create download directory: %v", err))
			return
		}
		// Update the download manager
		if a.downloadManager != nil {
			if err := a.downloadManager.SetDownloadDir(downloadDir); err != nil {
				respondError(w, http.StatusInternalServerError, err.Error())
				return
			}
		}
		logger.API("Updated Spotify download path to: %s", downloadDir)
	}

	if err := a.db.SetSetting(key, body.Value); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to save setting")
		return
	}

	respondJSON(w, map[string]string{"status": "ok"})
}
