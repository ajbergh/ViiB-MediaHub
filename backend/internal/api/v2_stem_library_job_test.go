package api

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/stems"
)

func TestStemLibraryScanRefreshesOnlyHashMatchingNestedPackages(t *testing.T) {
	root := t.TempDir()
	database, err := db.New(filepath.Join(root, "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	source := filepath.Join(root, "track.ogg")
	sourceBytes := []byte("source audio bytes")
	if err := os.WriteFile(source, sourceBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := database.SaveSong(&db.Song{ID: "local-track", Title: "Track", FilePath: source, AddedAt: 1}); err != nil {
		t.Fatal(err)
	}
	library := filepath.Join(root, "Stem Library")
	validPackage := filepath.Join(library, "Artist", "Album", "track.viibstems")
	wrongPackage := filepath.Join(library, "Artist", "Album", "other.viibstems")
	writeStemLibraryTestPackage(t, validPackage, sourceBytes)
	writeStemLibraryTestPackage(t, wrongPackage, []byte("different source"))
	if err := database.SetStemLocations([]db.StemLocation{{ID: "stem-root", Path: library, Enabled: true}}); err != nil {
		t.Fatal(err)
	}
	parameters, _ := json.Marshal(map[string][]string{"locations": {library}})
	api := &API{db: database}
	api.runStemLibraryScanJob(db.Job{ID: "scan", Type: "stem_library_scan", Parameters: parameters, Priority: 10})

	sets, err := database.ListStemSets("local-track")
	if err != nil {
		t.Fatal(err)
	}
	if len(sets) != 1 || sets[0].Status != "ready" || filepath.Clean(sets[0].PackagePath) != filepath.Clean(validPackage) {
		t.Fatalf("stem scan did not register only the matching nested package: %+v", sets)
	}
	songs, err := database.GetAllSongs()
	if err != nil {
		t.Fatal(err)
	}
	if len(songs) != 1 || songs[0].ID != "local-track" {
		t.Fatalf("stem scan changed the music catalog: %+v", songs)
	}
}

func TestStemLibraryScanRequiresASeparateConfiguredRoot(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	router := (&API{db: database}).V2StemRoutes()
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/scan", nil))
	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), "Stem Library location") {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestStemLibraryLocationsDoNotChangeMusicScanFolders(t *testing.T) {
	root := t.TempDir()
	database, err := db.New(filepath.Join(root, "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	musicRoot := filepath.Join(root, "Music")
	stemRoot := filepath.Join(root, "Stems")
	if err := os.MkdirAll(musicRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(stemRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := database.AddScanFolder(&db.ScanFolder{ID: "music", Path: musicRoot, AddedAt: 1}); err != nil {
		t.Fatal(err)
	}
	router := (&API{db: database}).V2StemRoutes()
	recorder := httptest.NewRecorder()
	requestBody, err := json.Marshal(map[string][]string{"locations": {stemRoot}})
	if err != nil {
		t.Fatal(err)
	}
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPut, "/locations", strings.NewReader(string(requestBody))))
	if recorder.Code != http.StatusOK {
		t.Fatalf("location save status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	folders, err := database.GetScanFolders()
	if err != nil {
		t.Fatal(err)
	}
	if len(folders) != 1 || filepath.Clean(folders[0].Path) != filepath.Clean(musicRoot) {
		t.Fatalf("Stem Library update changed Music Folders: %+v", folders)
	}
	locations, err := database.ListStemLocations()
	if err != nil {
		t.Fatal(err)
	}
	if len(locations) != 1 || filepath.Clean(locations[0].Path) != filepath.Clean(stemRoot) {
		t.Fatalf("Stem Library root was not stored separately: %+v", locations)
	}
}

func writeStemLibraryTestPackage(t *testing.T, dir string, source []byte) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	wav := stemLibraryTestWAV()
	wavHash := sha256.Sum256(wav)
	sourceHash := sha256.Sum256(source)
	manifest := stems.Manifest{
		SchemaVersion: 1,
		Source:        stems.Source{Filename: "track.ogg", SHA256: hex.EncodeToString(sourceHash[:]), Duration: 0.0005},
		StemLayout:    stems.LayoutFour,
		Generator:     stems.Provenance{Name: "Fixture", Version: "1"},
		Model:         stems.Provenance{Name: "Fixture", Version: "1"},
		Audio:         stems.AudioGeometry{SampleRate: 8000, Channels: 1, Frames: 4},
		Stems:         make(map[stems.StemName]stems.Artifact),
	}
	for _, name := range []stems.StemName{stems.StemVocals, stems.StemDrums, stems.StemBass, stems.StemOther} {
		artifactPath := string(name) + ".wav"
		if err := os.WriteFile(filepath.Join(dir, artifactPath), wav, 0o600); err != nil {
			t.Fatal(err)
		}
		manifest.Stems[name] = stems.Artifact{Path: artifactPath, SHA256: hex.EncodeToString(wavHash[:]), SizeBytes: int64(len(wav)), SampleRate: 8000, Channels: 1, Frames: 4, Encoding: "pcm_s16le"}
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, stems.ManifestFilename), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func stemLibraryTestWAV() []byte {
	const dataSize = 8
	wav := make([]byte, 44+dataSize)
	copy(wav[:4], "RIFF")
	binary.LittleEndian.PutUint32(wav[4:8], uint32(len(wav)-8))
	copy(wav[8:12], "WAVE")
	copy(wav[12:16], "fmt ")
	binary.LittleEndian.PutUint32(wav[16:20], 16)
	binary.LittleEndian.PutUint16(wav[20:22], 1)
	binary.LittleEndian.PutUint16(wav[22:24], 1)
	binary.LittleEndian.PutUint32(wav[24:28], 8000)
	binary.LittleEndian.PutUint32(wav[28:32], 16000)
	binary.LittleEndian.PutUint16(wav[32:34], 2)
	binary.LittleEndian.PutUint16(wav[34:36], 16)
	copy(wav[36:40], "data")
	binary.LittleEndian.PutUint32(wav[40:44], dataSize)
	return wav
}
