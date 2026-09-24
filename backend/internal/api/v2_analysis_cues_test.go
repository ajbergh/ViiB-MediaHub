package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/ajbergh/viib-mediahub/internal/analysis"
	"github.com/ajbergh/viib-mediahub/internal/analysis/beatgrid"
	"github.com/ajbergh/viib-mediahub/internal/analysis/features"
	"github.com/ajbergh/viib-mediahub/internal/db"
)

func TestV2AnalysisCueListAndApplyPolicies(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	sourcePath := filepath.Join(t.TempDir(), "song.mp3")
	if err := os.WriteFile(sourcePath, []byte("audio"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := database.SaveSong(&db.Song{ID: "song", Title: "Song", Artist: "Artist", Album: "Album", FilePath: sourcePath, Duration: 64, AddedAt: 1}); err != nil {
		t.Fatal(err)
	}
	resolvedSource, err := analysis.ResolveLocalSource(database, "song")
	if err != nil {
		t.Fatal(err)
	}
	if err := database.UpsertTrackAnalysis(db.TrackAnalysis{SongID: "song", Status: db.TrackAnalysisComplete, AnalysisVersion: 1, AlgorithmVersion: "test", SourceFingerprint: resolvedSource.Fingerprint}); err != nil {
		t.Fatal(err)
	}
	featureResult := features.Result{
		Energy:   []features.EnergyPoint{{Time: 0, Value: .2}, {Time: 32, Value: .8}},
		Sections: []features.Section{{Start: 0, End: 16, Energy: .2}, {Start: 16, End: 32, Energy: .8}, {Start: 32, End: 48, Energy: .3}, {Start: 48, End: 64, Energy: .7}},
	}
	energyData, err := featureResult.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if err := database.UpsertTrackAnalysisArtifact(db.TrackAnalysisArtifact{ID: "song:energy", SongID: "song", Kind: features.ArtifactKind, FormatVersion: features.FormatVersion, AlgorithmVersion: features.AlgorithmVersion, Encoding: features.Encoding, Data: energyData}); err != nil {
		t.Fatal(err)
	}
	grid, err := beatgrid.BuildStraight(120, 0, 64, 4)
	if err != nil {
		t.Fatal(err)
	}
	grid.Provenance = beatgrid.ProvenanceManual
	gridData, err := grid.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if err := database.UpsertTrackAnalysisArtifact(db.TrackAnalysisArtifact{ID: "song:grid", SongID: "song", Kind: beatgrid.ArtifactKind, FormatVersion: beatgrid.FormatVersion, AlgorithmVersion: beatgrid.AlgorithmVersion, Encoding: beatgrid.Encoding, Provenance: string(beatgrid.ProvenanceManual), Data: gridData}); err != nil {
		t.Fatal(err)
	}
	if err := database.SaveDJHotCues("song", []db.DJHotCue{{Slot: 1, Position: 1, Label: "User cue", Color: "#abcdef", Origin: "user"}}); err != nil {
		t.Fatal(err)
	}
	handler := (&API{db: database}).V2Routes()
	listRecorder := httptest.NewRecorder()
	handler.ServeHTTP(listRecorder, httptest.NewRequest(http.MethodGet, "/analysis/song/cues", nil))
	if listRecorder.Code != http.StatusOK {
		t.Fatalf("GET cue list = %d %s", listRecorder.Code, listRecorder.Body.String())
	}
	var listed AnalysisCueListResponse
	if err := json.NewDecoder(listRecorder.Body).Decode(&listed); err != nil {
		t.Fatal(err)
	}
	if listed.GeneratorVersion != "auto-cues-v1" || listed.SourceFingerprint != resolvedSource.Fingerprint || listed.DefaultApplyMode != "fill-empty" || len(listed.GeneratedCandidates) == 0 || len(listed.HotCues) != 1 {
		t.Fatalf("cue list response = %#v", listed)
	}
	for _, candidate := range listed.GeneratedCandidates {
		if !candidate.DownbeatAligned || candidate.Rationale != "qualified-manual-downbeat" || candidate.SourceFingerprint != resolvedSource.Fingerprint {
			t.Fatalf("manual beatgrid candidate provenance = %#v", candidate)
		}
	}

	applyRecorder := httptest.NewRecorder()
	handler.ServeHTTP(applyRecorder, httptest.NewRequest(http.MethodPost, "/analysis/song/cues/apply", bytes.NewBufferString(`{"mode":"fill-empty"}`)))
	if applyRecorder.Code != http.StatusOK {
		t.Fatalf("POST fill-empty = %d %s", applyRecorder.Code, applyRecorder.Body.String())
	}
	var applied AnalysisCueApplyResponse
	if err := json.NewDecoder(applyRecorder.Body).Decode(&applied); err != nil {
		t.Fatal(err)
	}
	if len(applied.AppliedSlots) == 0 || len(applied.BlockedSlots) == 0 || applied.HotCues[0].Origin != "user" {
		t.Fatalf("fill-empty result failed to preserve/block user slot: %#v", applied)
	}
	untouchedBefore := make(map[int]float64)
	for _, cue := range applied.HotCues {
		untouchedBefore[cue.Slot] = cue.Position
	}
	selectedRecorder := httptest.NewRecorder()
	handler.ServeHTTP(selectedRecorder, httptest.NewRequest(http.MethodPost, "/analysis/song/cues/apply", bytes.NewBufferString(`{"mode":"selected-only","selectedSlots":[2]}`)))
	if selectedRecorder.Code != http.StatusOK {
		t.Fatalf("POST selected-only = %d %s", selectedRecorder.Code, selectedRecorder.Body.String())
	}
	var selected AnalysisCueApplyResponse
	if err := json.NewDecoder(selectedRecorder.Body).Decode(&selected); err != nil {
		t.Fatal(err)
	}
	if len(selected.AppliedSlots) != 1 || selected.AppliedSlots[0] != 2 {
		t.Fatalf("selected-only applied slots = %v", selected.AppliedSlots)
	}
	for _, cue := range selected.HotCues {
		if cue.Slot != 2 && untouchedBefore[cue.Slot] != cue.Position {
			t.Fatalf("selected-only changed unrelated slot %d: %#v", cue.Slot, cue)
		}
	}
}

func TestV2AnalysisCueApplyValidatesModeAndSelection(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	handler := (&API{db: database}).V2Routes()
	for _, body := range []string{`{"mode":"replace-all"}`, `{"mode":"selected-only"}`, `{"mode":"selected-only","selectedSlots":[9]}`} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/analysis/missing/cues/apply", bytes.NewBufferString(body)))
		if recorder.Code != http.StatusBadRequest && recorder.Code != http.StatusNotFound {
			t.Fatalf("invalid apply %s = %d %s", body, recorder.Code, recorder.Body.String())
		}
	}
}
