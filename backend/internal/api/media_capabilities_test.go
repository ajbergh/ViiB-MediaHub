// Tests alignment between the canonical audio.DefaultCapabilityMatrix() and
// analysis.NewDefaultDecoderRegistry(), plus the /api/v2/capabilities/media
// endpoint and structured capability errors on /api/dj/waveform/{id}.
package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/ajbergh/viib-mediahub/internal/analysis"
	"github.com/ajbergh/viib-mediahub/internal/audio"
	"github.com/ajbergh/viib-mediahub/internal/db"
)

func TestMediaCapabilityMatrixMatchesAnalysisDecoderRegistry(t *testing.T) {
	registry := analysis.NewDefaultDecoderRegistry()
	matrix := audio.DefaultCapabilityMatrix()

	for _, f := range matrix.Formats {
		for _, ext := range f.Extensions {
			probeName := "fixture" + ext
			supportsDecode := registry.Supports(probeName)
			decoderID := registry.IDFor(probeName)

			if f.BackendAnalysis == audio.CapabilitySupported {
				if !supportsDecode {
					t.Fatalf("format %s (%s) claims BackendAnalysis=supported, but DecoderRegistry.Supports(%q) is false", f.ID, ext, probeName)
				}
				if decoderID != f.BackendDecoderID {
					t.Fatalf("format %s (%s) BackendDecoderID = %q, but DecoderRegistry.IDFor(%q) = %q", f.ID, ext, f.BackendDecoderID, probeName, decoderID)
				}
			} else {
				if supportsDecode {
					t.Fatalf("format %s (%s) has BackendAnalysis=%s, but DecoderRegistry.Supports(%q) is true", f.ID, ext, f.BackendAnalysis, probeName)
				}
			}
		}
	}
}

func TestGetMediaCapabilitiesV2AndDJWaveformCapabilityError(t *testing.T) {
	dir := t.TempDir()
	database, err := db.New(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })

	apiHandler := New(database, dir)
	t.Cleanup(apiHandler.Close)

	// 1. Verify GET /api/v2/capabilities/media
	v2Router := apiHandler.V2Routes()
	req := httptest.NewRequest(http.MethodGet, "/capabilities/media", nil)
	rec := httptest.NewRecorder()
	v2Router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /capabilities/media status = %d, want 200", rec.Code)
	}
	var matrix audio.CapabilityMatrix
	if err := json.Unmarshal(rec.Body.Bytes(), &matrix); err != nil {
		t.Fatalf("unmarshal capability matrix: %v", err)
	}
	if matrix.Version != audio.CapabilityMatrixVersion || len(matrix.Formats) < 7 {
		t.Fatalf("unexpected capability matrix response: %+v", matrix)
	}

	// 2. Verify /api/dj/waveform/{id} returns structured capability metadata on 422
	for _, ext := range []string{".flac", ".m4a", ".opus"} {
		filePath := filepath.Join(dir, "track"+ext)
		if err := os.WriteFile(filePath, []byte("placeholder-audio-bytes"), 0600); err != nil {
			t.Fatal(err)
		}
		songID := "song_" + ext[1:]
		if err := database.SaveSong(&db.Song{
			ID:       songID,
			Title:    "Track " + ext,
			Artist:   "Artist",
			Album:    "Album",
			Duration: 120,
			FilePath: filePath,
			AddedAt:  1,
		}); err != nil {
			t.Fatal(err)
		}

		apiRouter := apiHandler.Routes()
		wReq := httptest.NewRequest(http.MethodGet, "/dj/waveform/"+songID, nil)
		wRec := httptest.NewRecorder()
		apiRouter.ServeHTTP(wRec, wReq)

		if wRec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("GET /dj/waveform/%s status = %d, want 422 (body=%s)", songID, wRec.Code, wRec.Body.String())
		}
		var payload struct {
			Error      string                `json:"error"`
			Capability audio.CapabilityError `json:"capability"`
		}
		if err := json.Unmarshal(wRec.Body.Bytes(), &payload); err != nil {
			t.Fatalf("unmarshal 422 payload for %s: %v", ext, err)
		}
		if payload.Capability.Code != audio.CapabilityErrorClientWaveformFallback || !payload.Capability.FallbackAvailable || payload.Capability.Extension != ext {
			t.Fatalf("unexpected capability error for %s: %+v", ext, payload.Capability)
		}
	}
}
