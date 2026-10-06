package api

import (
	"encoding/json"
	"github.com/ajbergh/viib-mediahub/internal/analysis"
	"github.com/ajbergh/viib-mediahub/internal/analysis/threeband"
	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/go-chi/chi/v5"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestLocalThreeBandReadFencesSourceAndPayload(t *testing.T) {
	a, path := newBPMRouteTestAPI(t, false)
	source, err := analysis.ResolveLocalSource(a.db, "song")
	if err != nil {
		t.Fatal(err)
	}
	acc, _ := threeband.New(48000)
	_ = acc.Feed([]float32{0, .5, -.5})
	o, _ := acc.Result()
	raw, _ := threeband.Encode(o)
	artifact := db.TrackAnalysisArtifact{ID: "bands", SongID: "song", Kind: threeband.Kind, FormatVersion: threeband.FormatVersion, AlgorithmVersion: threeband.AlgorithmVersion, Encoding: threeband.Encoding, Provenance: "measured", SourceFingerprint: source.Fingerprint, Data: raw}
	router := chi.NewRouter()
	router.Get("/analysis/{songID}/waveform/local-three-band", a.getLocalThreeBand)
	call := func() *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/analysis/song/waveform/local-three-band", nil))
		return w
	}
	if call().Code != 404 {
		t.Fatal("unprepared waveform returned")
	}
	if err := a.db.UpsertTrackAnalysisArtifact(artifact); err != nil {
		t.Fatal(err)
	}
	w := call()
	var response struct {
		Representation string             `json:"representation"`
		Overview       threeband.Overview `json:"overview"`
	}
	if json.Unmarshal(w.Body.Bytes(), &response) != nil || w.Code != 200 || response.Representation != threeband.Kind || response.Overview.Frames != 3 || w.Header().Get("ETag") == "" {
		t.Fatalf("waveform response: %d %s", w.Code, w.Body.String())
	}
	artifact.Data = []byte("broken")
	if err := a.db.UpsertTrackAnalysisArtifact(artifact); err != nil {
		t.Fatal(err)
	}
	if call().Code != 500 {
		t.Fatal("corrupt artifact returned")
	}
	artifact.Data = raw
	artifact.SourceFingerprint = ""
	if err := a.db.UpsertTrackAnalysisArtifact(artifact); err != nil {
		t.Fatal(err)
	}
	if call().Code != 404 {
		t.Fatal("unbound artifact returned")
	}
	artifact.SourceFingerprint = source.Fingerprint
	if err := a.db.UpsertTrackAnalysisArtifact(artifact); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("changed local file content"), 0600); err != nil {
		t.Fatal(err)
	}
	if call().Code != 404 {
		t.Fatal("old waveform returned for replaced bytes")
	}
}
