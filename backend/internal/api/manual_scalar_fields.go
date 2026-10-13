package api

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/go-chi/chi/v5"
)

func (a *API) putManualScalarV2(w http.ResponseWriter, r *http.Request) {
	a.writeManualScalarV2(w, r, false)
}
func (a *API) resetManualScalarV2(w http.ResponseWriter, r *http.Request) {
	a.writeManualScalarV2(w, r, true)
}
func (a *API) writeManualScalarV2(w http.ResponseWriter, r *http.Request, reset bool) {
	song, key := chi.URLParam(r, "songID"), chi.URLParam(r, "fieldKey")
	var update struct {
		Value json.RawMessage `json:"value"`
	}
	if !reset {
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&update); err != nil {
			respondError(w, 400, "invalid manual field update")
			return
		}
		if err := decoder.Decode(&struct{}{}); err != io.EOF {
			respondError(w, 400, "invalid manual field update")
			return
		}
	} else {
		update.Value = json.RawMessage("4")
	}
	if _, err := db.ManualScalarCandidate(key, "validation", update.Value, time.Now().UnixMilli()); err != nil {
		respondError(w, 400, err.Error())
		return
	}
	fp, err := a.requireCurrentAnalysisSource(w, r, song)
	if err != nil {
		return
	}
	changed, err := a.db.SetTrackMetadataOverrideIfSourceCurrent(song, key, fp, update.Value, reset)
	if err != nil {
		respondError(w, 500, "manual field unavailable")
		return
	}
	if !changed {
		respondError(w, 412, "song source changed; reload analysis details")
		return
	}
	if err := a.revalidateAnalysisSourceAfterWrite(song, fp); err != nil {
		respondError(w, 412, "song source changed; reload analysis details")
		return
	}
	w.Header().Set("ETag", strconv.Quote(fp))
	w.WriteHeader(http.StatusNoContent)
}
