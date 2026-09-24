package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/ajbergh/viib-mediahub/internal/db"
)

func TestSaveDJHotCuesRejectsInvalidSlotsAndRoundTripsProvenance(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.SaveSong(&db.Song{ID: "song", Title: "Song", Artist: "Artist", Album: "Album", FilePath: "song.mp3", AddedAt: 1}); err != nil {
		t.Fatal(err)
	}
	server := (&API{db: database}).Routes()

	invalid := httptest.NewRequest(http.MethodPut, "/dj/hotcues/song", bytes.NewBufferString(`{"hotCues":[{"slot":0,"position":4,"color":"#fff"}]}`))
	invalidResult := httptest.NewRecorder()
	server.ServeHTTP(invalidResult, invalid)
	if invalidResult.Code != http.StatusBadRequest {
		t.Fatalf("invalid slot response = %d %s, want 400", invalidResult.Code, invalidResult.Body.String())
	}
	if cues, err := database.GetDJHotCues("song"); err != nil || len(cues) != 0 {
		t.Fatalf("cues after invalid slot save = %#v, %v; want no rows", cues, err)
	}
	unqualifiedDownbeat := httptest.NewRequest(http.MethodPut, "/dj/hotcues/song", bytes.NewBufferString(`{"hotCues":[{"slot":1,"position":1,"origin":"analysis","generatorVersion":"cue-v1","confidence":0.5,"kind":"section","downbeatAligned":true,"rationale":"nearest-beat-inferred-from-meter"}]}`))
	unqualifiedResult := httptest.NewRecorder()
	server.ServeHTTP(unqualifiedResult, unqualifiedDownbeat)
	if unqualifiedResult.Code != http.StatusBadRequest {
		t.Fatalf("unqualified downbeat claim response = %d %s, want 400", unqualifiedResult.Code, unqualifiedResult.Body.String())
	}

	body := `{"hotCues":[{"slot":4,"position":14.25,"label":"Drop","color":"#112233","origin":"analysis","generatorVersion":"cue-v2","confidence":0.91,"kind":"drop","locked":true,"rationale":"qualified-measured-downbeat","sourceFingerprint":"audio-sha256","downbeatAligned":true,"updatedAt":1725000000123}]}`
	request := httptest.NewRequest(http.MethodPut, "/dj/hotcues/song", bytes.NewBufferString(body))
	result := httptest.NewRecorder()
	server.ServeHTTP(result, request)
	if result.Code != http.StatusOK {
		t.Fatalf("save response = %d %s, want 200", result.Code, result.Body.String())
	}
	getRequest := httptest.NewRequest(http.MethodGet, "/dj/hotcues/song", nil)
	getResult := httptest.NewRecorder()
	server.ServeHTTP(getResult, getRequest)
	if getResult.Code != http.StatusOK {
		t.Fatalf("get response = %d %s, want 200", getResult.Code, getResult.Body.String())
	}
	wantFields := []string{`"origin":"analysis"`, `"generatorVersion":"cue-v2"`, `"confidence":0.91`, `"kind":"drop"`, `"locked":true`, `"rationale":"qualified-measured-downbeat"`, `"sourceFingerprint":"audio-sha256"`, `"downbeatAligned":true`, `"updatedAt":1725000000123`}
	for _, field := range wantFields {
		if !bytes.Contains(getResult.Body.Bytes(), []byte(field)) {
			t.Errorf("GET response %s missing %s", getResult.Body.String(), field)
		}
	}
}
