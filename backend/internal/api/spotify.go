package api

import (
	"encoding/json"
	"net/http"
)

type SpotifyCredentials struct {
	ClientId     string `json:"clientId"`
	ClientSecret string `json:"clientSecret"`
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	Expiry       int64  `json:"expiry"`
}

func (a *API) saveSpotifyCredentials(w http.ResponseWriter, r *http.Request) {
	var creds SpotifyCredentials
	if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Save to DB settings
	// We'll store as a JSON string for simplicity, or individual keys
	// Let's store individual keys for easier access if needed, or JSON for grouping.
	// JSON is cleaner for "credentials" object.

	credsJSON, err := json.Marshal(creds)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to marshal credentials")
		return
	}

	if err := a.db.SetSetting("spotify_credentials", string(credsJSON)); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to save credentials")
		return
	}

	respondJSON(w, map[string]string{"status": "ok"})
}

func (a *API) getSpotifyCredentials(w http.ResponseWriter, r *http.Request) {
	val, err := a.db.GetSetting("spotify_credentials")
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to retrieve credentials")
		return
	}

	if val == "" {
		respondJSON(w, map[string]interface{}{}) // Empty object if not set
		return
	}

	var creds SpotifyCredentials
	if err := json.Unmarshal([]byte(val), &creds); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to parse credentials")
		return
	}

	respondJSON(w, creds)
}
