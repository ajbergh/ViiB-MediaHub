package api

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/scanner"
)

func (a *API) getDuplicateGroups(w http.ResponseWriter, r *http.Request) {
	groups, err := a.db.GetDuplicateGroups()
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if groups == nil {
		groups = []db.DuplicateGroup{}
	}
	respondJSON(w, groups)
}

func (a *API) getIgnoredSongs(w http.ResponseWriter, r *http.Request) {
	songs, err := a.db.GetIgnoredSongs()
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for i := range songs {
		transformLibrarySongForAPI(&songs[i])
	}
	respondJSON(w, songs)
}

func (a *API) setDuplicateIgnored(w http.ResponseWriter, r *http.Request) {
	var request struct {
		SongID  string `json:"songId"`
		Ignored bool   `json:"ignored"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.SongID == "" {
		respondError(w, http.StatusBadRequest, "songId is required")
		return
	}
	if err := a.db.SetSongIgnored(request.SongID, request.Ignored); err != nil {
		if err == sql.ErrNoRows {
			respondError(w, http.StatusNotFound, "Song not found")
		} else {
			respondError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	if a.scanner != nil {
		a.scanner.EmitEvent(scanner.LibraryEvent{Type: "library_updated", Message: "Duplicate visibility updated"})
	}
	respondJSON(w, map[string]any{"status": "ok", "ignored": request.Ignored})
}
