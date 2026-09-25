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

func TestStemLibraryScanKeepsRegisteredMismatchesStaleAndInvalidPackagesInvalid(t *testing.T) {
	root := t.TempDir()
	database, err := db.New(filepath.Join(root, "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	sourceBytes := []byte("current track source")
	source := filepath.Join(root, "track.ogg")
	if err := os.WriteFile(source, sourceBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := database.SaveSong(&db.Song{ID: "local-track", Title: "Track", FilePath: source, AddedAt: 1}); err != nil {
		t.Fatal(err)
	}
	library := filepath.Join(root, "Stem Library")
	matching := filepath.Join(library, "Artist", "Album", "matching.viibstems")
	mismatched := filepath.Join(library, "Artist", "Album", "mismatched.viibstems")
	invalid := filepath.Join(library, "Artist", "Album", "invalid.viibstems")
	writeStemLibraryTestPackage(t, matching, sourceBytes)
	writeStemLibraryTestPackage(t, mismatched, []byte("older source"))
	writeStemLibraryTestPackage(t, invalid, sourceBytes)
	invalidManifest := filepath.Join(invalid, stems.ManifestFilename)
	if err := os.WriteFile(invalidManifest, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	for id, path := range map[string]string{"mismatched-set": mismatched, "invalid-set": invalid} {
		validation, validationErr := stems.ValidatePackage(path)
		if validationErr != nil && id == "mismatched-set" {
			t.Fatal(validationErr)
		}
		set := db.StemSet{ID: id, SongID: "local-track", PackagePath: path, DiscoverySource: "library", Status: "ready", SourceAudioHash: "previously-valid"}
		if validationErr == nil {
			set = stemSetFromValidation("local-track", path, "library", validation.Manifest, validation.Files, false, "ready", "")
			set.ID = id
		} else {
			// Preserve a formerly valid registry record after its on-disk package
			// becomes corrupt; the next scan must classify it as invalid.
			set.ID, set.SongID, set.PackagePath, set.DiscoverySource, set.Status = id, "local-track", path, "library", "ready"
		}
		if err := database.UpsertStemSet(set); err != nil {
			t.Fatal(err)
		}
	}
	if err := database.SetStemLocations([]db.StemLocation{{ID: "stem-root", Path: library, Enabled: true}}); err != nil {
		t.Fatal(err)
	}
	parameters, _ := json.Marshal(map[string][]string{"locations": {library}})
	(&API{db: database}).runStemLibraryScanJob(db.Job{ID: "scan", Type: "stem_library_scan", Parameters: parameters, Priority: 10})

	sets, err := database.ListStemSets("local-track")
	if err != nil {
		t.Fatal(err)
	}
	statuses := make(map[string]string, len(sets))
	for _, set := range sets {
		statuses[filepath.Clean(set.PackagePath)] = set.Status
	}
	if statuses[filepath.Clean(matching)] != "ready" || statuses[filepath.Clean(mismatched)] != "stale" || statuses[filepath.Clean(invalid)] != "invalid" {
		t.Fatalf("unexpected registry statuses after scan: %+v", sets)
	}
}

func TestGenericJobsCannotCreateArbitraryStemLibraryScans(t *testing.T) {
	root := t.TempDir()
	database, err := db.New(filepath.Join(root, "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	arbitrary := filepath.Join(root, "outside-configured-roots")
	if err := os.MkdirAll(arbitrary, 0o700); err != nil {
		t.Fatal(err)
	}
	router := (&API{db: database}).V2JobRoutes()
	body, _ := json.Marshal(map[string]any{"type": "stem_library_scan", "parameters": map[string][]string{"locations": {arbitrary}}})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(string(body))))
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "configured locations") {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	jobs, err := database.ListJobs(10, "")
	if err != nil || len(jobs) != 0 {
		t.Fatalf("generic request created a Stem Library job: jobs=%+v err=%v", jobs, err)
	}
}

func TestStemLibraryWorkerRejectsRootsDisabledAfterQueueing(t *testing.T) {
	root := t.TempDir()
	database, err := db.New(filepath.Join(root, "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	library := filepath.Join(root, "Stem Library")
	if err := os.MkdirAll(library, 0o700); err != nil {
		t.Fatal(err)
	}
	parameters, _ := json.Marshal(map[string][]string{"locations": {library}})
	job := db.Job{ID: "scan", Type: "stem_library_scan", Status: db.JobStatusQueued, Parameters: parameters}
	if err := database.CreateJob(job); err != nil {
		t.Fatal(err)
	}
	if err := database.StartJob(job.ID, "Running"); err != nil {
		t.Fatal(err)
	}
	(&API{db: database}).runStemLibraryScanJob(job)
	completed, err := database.GetJob(job.ID)
	if err != nil || completed.Status != db.JobStatusFailed || completed.ErrorCode != "stem_library_roots_invalid" {
		t.Fatalf("queued roots removed before execution were not rejected: %+v err=%v", completed, err)
	}
}

func TestStemLibraryScanCancellationBeforeEmptyCatalogCompletion(t *testing.T) {
	root := t.TempDir()
	database, err := db.New(filepath.Join(root, "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	library := filepath.Join(root, "Stem Library")
	if err := os.MkdirAll(library, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := database.SetStemLocations([]db.StemLocation{{ID: "stem-root", Path: library, Enabled: true}}); err != nil {
		t.Fatal(err)
	}
	parameters, _ := json.Marshal(map[string][]string{"locations": {library}})
	job := db.Job{ID: "scan", Type: "stem_library_scan", Status: db.JobStatusQueued, Parameters: parameters}
	if err := database.CreateJob(job); err != nil {
		t.Fatal(err)
	}
	if err := database.StartJob(job.ID, "Running"); err != nil {
		t.Fatal(err)
	}
	if changed, cancelErr := database.RequestJobCancellation(job.ID); cancelErr != nil || !changed {
		t.Fatalf("request cancellation: changed=%v err=%v", changed, cancelErr)
	}
	(&API{db: database}).runStemLibraryScanJob(job)
	completed, err := database.GetJob(job.ID)
	if err != nil || completed.Status != db.JobStatusCanceled {
		t.Fatalf("empty scan overwrote cancellation: status=%s err=%v", completed.Status, err)
	}
}

func TestExplicitSymlinkPackageRefreshMarksSetInvalid(t *testing.T) {
	root := t.TempDir()
	database, err := db.New(filepath.Join(root, "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	source := filepath.Join(root, "track.ogg")
	sourceBytes := []byte("current source")
	if err := os.WriteFile(source, sourceBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := database.SaveSong(&db.Song{ID: "song", Title: "Track", FilePath: source, AddedAt: 1}); err != nil {
		t.Fatal(err)
	}
	realPackage := filepath.Join(root, "real.viibstems")
	writeStemLibraryTestPackage(t, realPackage, sourceBytes)
	alias := filepath.Join(root, "linked.viibstems")
	if err := os.Symlink(realPackage, alias); err != nil {
		t.Skipf("symlink creation unavailable: %v", err)
	}
	if err := database.UpsertStemSet(db.StemSet{ID: "linked", SongID: "song", PackagePath: alias, DiscoverySource: "explicit", Status: "ready", ExplicitlyLinked: true}); err != nil {
		t.Fatal(err)
	}
	if err := (&API{db: database}).refreshStemRegistry("song"); err != nil {
		t.Fatal(err)
	}
	sets, err := database.ListStemSets("song")
	if err != nil || len(sets) != 1 || sets[0].Status != "invalid" || !sets[0].ExplicitlyLinked {
		t.Fatalf("symlinked explicit package remained playable: sets=%+v err=%v", sets, err)
	}
}

func TestExplicitLinkJobRejectsSymlinkPackageRoot(t *testing.T) {
	root := t.TempDir()
	database, err := db.New(filepath.Join(root, "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	source := filepath.Join(root, "track.ogg")
	sourceBytes := []byte("current source")
	if err := os.WriteFile(source, sourceBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := database.SaveSong(&db.Song{ID: "song", Title: "Track", FilePath: source, AddedAt: 1}); err != nil {
		t.Fatal(err)
	}
	realPackage := filepath.Join(root, "real.viibstems")
	writeStemLibraryTestPackage(t, realPackage, sourceBytes)
	alias := filepath.Join(root, "linked.viibstems")
	if err := os.Symlink(realPackage, alias); err != nil {
		t.Skipf("symlink creation unavailable: %v", err)
	}
	parameters, _ := json.Marshal(map[string]string{"songId": "song", "packagePath": alias})
	job := db.Job{ID: "link", Type: "stem_package_link", Status: db.JobStatusQueued, Parameters: parameters, Priority: 100}
	if err := database.CreateJob(job); err != nil {
		t.Fatal(err)
	}
	if err := database.StartJob(job.ID, "Running"); err != nil {
		t.Fatal(err)
	}
	(&API{db: database}).runStemRegistryJob(job)
	completed, err := database.GetJob(job.ID)
	if err != nil || completed.Status != db.JobStatusFailed || completed.ErrorCode != "invalid_stem_package" {
		t.Fatalf("symlink link job status=%+v err=%v", completed, err)
	}
	sets, err := database.ListStemSets("song")
	if err != nil || len(sets) != 0 {
		t.Fatalf("symlink package was registered: sets=%+v err=%v", sets, err)
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

func TestStemLibraryRootsCannotOverlapMusicFolderRoots(t *testing.T) {
	tests := []struct {
		name      string
		musicRel  string
		stemsRel  string
		wantError bool
	}{
		{name: "equal", musicRel: "Music", stemsRel: "Music", wantError: true},
		{name: "stem child of music", musicRel: "Music", stemsRel: filepath.Join("Music", "Stems"), wantError: true},
		{name: "music child of stem", musicRel: filepath.Join("Stems", "Music"), stemsRel: "Stems", wantError: true},
		{name: "sibling prefix is separate", musicRel: "Music", stemsRel: "Musical", wantError: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			database, err := db.New(filepath.Join(root, "library.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer database.Close()
			musicRoot := filepath.Join(root, test.musicRel)
			stemRoot := filepath.Join(root, test.stemsRel)
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
			body, _ := json.Marshal(map[string][]string{"locations": {stemRoot}})
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPut, "/locations", strings.NewReader(string(body))))
			if test.wantError && (recorder.Code != http.StatusBadRequest || !strings.Contains(strings.ToLower(recorder.Body.String()), "overlaps")) {
				t.Fatalf("overlap status=%d body=%s", recorder.Code, recorder.Body.String())
			}
			if !test.wantError && recorder.Code != http.StatusOK {
				t.Fatalf("sibling roots rejected: status=%d body=%s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestMusicFolderCannotBeAddedInsideStemLibraryRoot(t *testing.T) {
	root := t.TempDir()
	database, err := db.New(filepath.Join(root, "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	stemRoot := filepath.Join(root, "Stems")
	musicRoot := filepath.Join(stemRoot, "Music")
	if err := os.MkdirAll(musicRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := database.SetStemLocations([]db.StemLocation{{ID: "stem-root", Path: stemRoot, Enabled: true}}); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]string{"path": musicRoot})
	recorder := httptest.NewRecorder()
	(&API{db: database}).addScanFolder(recorder, httptest.NewRequest(http.MethodPost, "/folders", strings.NewReader(string(body))))
	if recorder.Code != http.StatusBadRequest || !strings.Contains(strings.ToLower(recorder.Body.String()), "overlaps") {
		t.Fatalf("music folder overlap status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestStemLibraryScanRejectsLegacyOverlappingConfiguration(t *testing.T) {
	root := t.TempDir()
	database, err := db.New(filepath.Join(root, "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	musicRoot := filepath.Join(root, "Music")
	stemRoot := filepath.Join(musicRoot, "Stems")
	if err := os.MkdirAll(stemRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := database.AddScanFolder(&db.ScanFolder{ID: "music", Path: musicRoot, AddedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if err := database.SetStemLocations([]db.StemLocation{{ID: "legacy-stem-root", Path: stemRoot, Enabled: true}}); err != nil {
		t.Fatal(err)
	}
	router := (&API{db: database}).V2StemRoutes()
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/scan", nil))
	if recorder.Code != http.StatusConflict || !strings.Contains(strings.ToLower(recorder.Body.String()), "overlaps") {
		t.Fatalf("legacy overlap was scanned: status=%d body=%s", recorder.Code, recorder.Body.String())
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
