package api

import (
	"bytes"
	"encoding/json"
	"github.com/ajbergh/viib-mediahub/internal/analysis/beatgrid"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
)

func TestBeatGridSourceFenceAndUnresolvedLock(t *testing.T) {
	a, _ := newBPMRouteTestAPI(t, false)
	handler := a.V2Routes()
	fps, err := a.currentAnalysisSourceFingerprints([]string{"song"})
	if err != nil {
		t.Fatal(err)
	}
	fp := fps["song"]
	body := `{"beats":[0,0.5,1],"downbeatIndices":[0],"locked":true}`
	request := func(method, token string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, "/analysis/song/beatgrid", strings.NewReader(body))
		if token != "" {
			r.Header.Set("If-Match", strconv.Quote(token))
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	if w := request(http.MethodPut, ""); w.Code != 428 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := request(http.MethodPut, fp); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	before, err := a.db.GetTrackAnalysisArtifact("song", beatgrid.ArtifactKind, beatgrid.FormatVersion, beatgrid.AlgorithmVersion)
	if err != nil || before.SourceFingerprint != fp {
		t.Fatal(before, err)
	}
	song, err := a.db.GetSongByID("song")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(song.FilePath, []byte("changed beatgrid source"), 0600); err != nil {
		t.Fatal(err)
	}
	w := request(http.MethodGet, "")
	var grid BeatGridResponse
	if err := json.Unmarshal(w.Body.Bytes(), &grid); err != nil || w.Code != 200 || grid.Resolution != "unavailable" || grid.Reason != "source_mismatch" || !grid.Locked || len(grid.Beats) != 0 || grid.SourceFingerprint == fp {
		t.Fatal(grid, w.Code, err)
	}
	if w := request(http.MethodPut, fp); w.Code != 412 {
		t.Fatal(w.Code, w.Body.String())
	}
	after, err := a.db.GetTrackAnalysisArtifact("song", beatgrid.ArtifactKind, beatgrid.FormatVersion, beatgrid.AlgorithmVersion)
	if err != nil || !bytes.Equal(before.Data, after.Data) || after.SourceFingerprint != fp {
		t.Fatal("stale save mutated artifact", err)
	}
	if w := request(http.MethodDelete, fp); w.Code != 412 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := request(http.MethodDelete, grid.SourceFingerprint); w.Code != 204 {
		t.Fatal(w.Code, w.Body.String())
	}
}
