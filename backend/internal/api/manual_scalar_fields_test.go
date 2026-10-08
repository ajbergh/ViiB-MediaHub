package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/ajbergh/viib-mediahub/internal/analysis"
	"github.com/ajbergh/viib-mediahub/internal/analysis/features"
	"github.com/ajbergh/viib-mediahub/internal/db"
)

func TestManualScalarFieldsSourceFenceAndDetailListParity(t *testing.T) {
	a, path := newBPMRouteTestAPI(t, false)
	source, err := analysis.ResolveLocalSource(a.db, "song")
	if err != nil {
		t.Fatal(err)
	}
	router := a.V2Routes()
	write := func(method, key, body, fp string) int {
		request := httptest.NewRequest(method, "/analysis/song/fields/"+key, strings.NewReader(body))
		if fp != "" {
			request.Header.Set("If-Match", strconv.Quote(fp))
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, request)
		return w.Code
	}
	if code := write(http.MethodPut, "time_signature", `{"value":3}`, ""); code != 428 {
		t.Fatal(code)
	}
	if code := write(http.MethodPut, "time_signature", `{"value":3}`, source.Fingerprint); code != 204 {
		t.Fatal(code)
	}
	if code := write(http.MethodPut, "local_energy_level", `{"value":7}`, source.Fingerprint); code != 204 {
		t.Fatal(code)
	}
	for _, tc := range []struct{ key, body string }{{"time_signature", `{"value":null}`}, {"time_signature", `{"value":3.5}`}, {"local_energy_level", `{"value":11}`}, {"spotify_energy_score", `{"value":0}`}, {"local_duration_seconds", `{"value":12}`}, {"time_signature", `{"value":4,"units":"seconds"}`}} {
		if code := write(http.MethodPut, tc.key, tc.body, source.Fingerprint); code != 400 {
			t.Fatalf("invalid %s %s = %d", tc.key, tc.body, code)
		}
	}
	check := func(row TrackAnalysisFeatureResponse) {
		if row.EnergyLevel == nil || *row.EnergyLevel != 7 || row.EnergyLevelSource != "manual" || row.EnergyLevelConfidence != nil || row.EnergyAlgorithmVersion != nil {
			t.Fatalf("manual compatibility energy: %+v", row)
		}
		values := map[string]string{}
		for _, field := range row.EffectiveFields {
			if field.Selected != nil && field.Selected.Source == "manual" {
				values[field.Key] = string(field.Selected.Value)
			}
		}
		if values["time_signature"] != "3" || values["local_energy_level"] != "7" {
			t.Fatalf("manual effective values: %+v", values)
		}
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/analysis/song", nil))
	var detail TrackAnalysisFeatureResponse
	if err := json.Unmarshal(w.Body.Bytes(), &detail); err != nil || w.Code != 200 {
		t.Fatalf("detail: %d %s", w.Code, w.Body.String())
	}
	check(detail)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/analysis", nil))
	var rows []TrackAnalysisFeatureResponse
	if err := json.Unmarshal(w.Body.Bytes(), &rows); err != nil || w.Code != 200 {
		t.Fatalf("list: %d %s", w.Code, w.Body.String())
	}
	for _, row := range rows {
		if row.SongID == "song" {
			check(row)
		}
	}
	if code := write(http.MethodDelete, "time_signature", "", source.Fingerprint); code != 204 {
		t.Fatal(code)
	}
	fields, err := a.db.GetTrackMetadataOverrides("song")
	if err != nil || len(fields) != 1 || fields[0].Key != "local_energy_level" {
		t.Fatalf("reset changed sibling: %+v %v", fields, err)
	}
	if err := os.WriteFile(path, []byte("new source bytes and length"), 0600); err != nil {
		t.Fatal(err)
	}
	if code := write(http.MethodDelete, "local_energy_level", "", source.Fingerprint); code != 412 {
		t.Fatal(code)
	}
	fields, err = a.db.GetTrackMetadataOverrides("song")
	if err != nil || len(fields) != 1 {
		t.Fatal("stale reset erased data")
	}
	if got := db.ResolveEffectiveScalar("local_energy_level", "changed", fields); got.Selected != nil {
		t.Fatal("old manual value applied to replacement")
	}
}

func TestExplicitMissingEnergyIsUnknownInDetailAndList(t *testing.T) {
	a, _ := newBPMRouteTestAPI(t, false)
	source, err := analysis.ResolveLocalSource(a.db, "song")
	if err != nil {
		t.Fatal(err)
	}
	level, confidence, version := 8, .8, features.EnergyLevelAlgorithmVersion
	record := db.TrackAnalysis{SongID: "song", Status: db.TrackAnalysisComplete, AnalysisVersion: 1, AlgorithmVersion: "v1", SourceFingerprint: source.Fingerprint, EnergyLevel: &level, EnergyLevelConfidence: &confidence, EnergyAlgorithmVersion: &version, Local: &db.LocalScalarObservation{SourceFingerprint: source.Fingerprint, AlgorithmVersion: "v1"}}
	if err := a.db.UpsertTrackAnalysis(record); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/analysis/song", "/analysis"} {
		w := httptest.NewRecorder()
		a.V2Routes().ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 200 {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
		var rows []TrackAnalysisFeatureResponse
		if path == "/analysis" {
			if err := json.Unmarshal(w.Body.Bytes(), &rows); err != nil {
				t.Fatal(err)
			}
		} else {
			var row TrackAnalysisFeatureResponse
			if err := json.Unmarshal(w.Body.Bytes(), &row); err != nil {
				t.Fatal(err)
			}
			rows = append(rows, row)
		}
		found := false
		for _, row := range rows {
			if row.SongID == "song" {
				found = true
				if row.EnergyLevel != nil || row.MeasuredEnergyLevel != nil {
					t.Fatalf("%s leaked compatibility: %+v", path, row)
				}
			}
		}
		if !found {
			t.Fatalf("%s omitted song", path)
		}
	}
}

func TestLegacyEnergyCanonicalDetailListParity(t *testing.T) {
	a, _ := newBPMRouteTestAPI(t, false)
	source, err := analysis.ResolveLocalSource(a.db, "song")
	if err != nil {
		t.Fatal(err)
	}
	level, confidence, version := 6, .8, features.EnergyLevelAlgorithmVersion
	record := db.TrackAnalysis{SongID: "song", Status: db.TrackAnalysisComplete, AnalysisVersion: 1, AlgorithmVersion: "v1", SourceFingerprint: source.Fingerprint, EnergyLevel: &level, EnergyLevelConfidence: &confidence, EnergyAlgorithmVersion: &version}
	if err := a.db.UpsertTrackAnalysis(record); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/analysis/song", "/analysis"} {
		w := httptest.NewRecorder()
		a.V2Routes().ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 200 {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
		var rows []TrackAnalysisFeatureResponse
		if path == "/analysis" {
			if err := json.Unmarshal(w.Body.Bytes(), &rows); err != nil {
				t.Fatal(err)
			}
		} else {
			var row TrackAnalysisFeatureResponse
			if err := json.Unmarshal(w.Body.Bytes(), &row); err != nil {
				t.Fatal(err)
			}
			rows = append(rows, row)
		}
		found := false
		for _, row := range rows {
			if row.SongID != "song" {
				continue
			}
			found = true
			var selected *db.ScalarCandidate
			for _, field := range row.EffectiveFields {
				if field.Key == "local_energy_level" {
					selected = field.Selected
				}
			}
			if selected == nil || selected.Source != "local" || string(selected.Value) != "6" || row.EnergyLevel == nil || *row.EnergyLevel != 6 || row.MeasuredEnergyLevel == nil || *row.MeasuredEnergyLevel != 6 {
				t.Fatalf("%s energy disagreement: %+v", path, row)
			}
		}
		if !found {
			t.Fatal("song omitted")
		}
	}
}
