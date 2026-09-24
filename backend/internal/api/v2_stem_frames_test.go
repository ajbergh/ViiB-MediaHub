package api

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ajbergh/viib-mediahub/internal/analysis"
	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/stems"
)

func TestV2StemFramesPacksCommonFrameRangeInDJ4Order(t *testing.T) {
	fixture := newStemFrameFixture(t, stems.LayoutFour)
	recorder := fixture.request("/song/set/frames?startFrame=1&frameCount=2&layout=dj4&format=f32le")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if recorder.Header().Get("X-Start-Frame") != "1" || recorder.Header().Get("X-Frame-Count") != "2" || recorder.Header().Get("X-Channel-Order") != "vocals,drums,bass,music" {
		t.Fatalf("unexpected alignment headers: %v", recorder.Header())
	}
	if got, want := recorder.Body.Len(), 2*4*2*4; got != want {
		t.Fatalf("got %d packed bytes, want %d", got, want)
	}
	samples := make([]float32, recorder.Body.Len()/4)
	for i := range samples {
		samples[i] = math.Float32frombits(binary.LittleEndian.Uint32(recorder.Body.Bytes()[i*4:]))
	}
	for i, want := range []float32{.11, .111, .21, .211, .31, .311, .41, .411} {
		if math.Abs(float64(samples[i]-want)) > .002 {
			t.Fatalf("packed sample %d = %f, want %f", i, samples[i], want)
		}
	}
}

func TestV2StemFramesSixPackageDJ4MixAndSixS16(t *testing.T) {
	fixture := newStemFrameFixture(t, stems.LayoutSix)
	dj4 := fixture.request("/song/set/frames?startFrame=0&frameCount=1&layout=dj4&format=f32le")
	if dj4.Code != http.StatusOK {
		t.Fatalf("dj4 status=%d body=%s", dj4.Code, dj4.Body.String())
	}
	musicLeft := math.Float32frombits(binary.LittleEndian.Uint32(dj4.Body.Bytes()[6*4:]))
	if math.Abs(float64(musicLeft-.15)) > .003 {
		t.Fatalf("music bus sample=%f, want sum .15", musicLeft)
	}
	six := fixture.request("/song/set/frames?startFrame=2&frameCount=1&layout=six&format=s16le")
	if six.Code != http.StatusOK {
		t.Fatalf("six status=%d body=%s", six.Code, six.Body.String())
	}
	if six.Body.Len() != 6*2*2 || six.Header().Get("X-Channel-Order") != "vocals,drums,bass,guitar,piano,other" {
		t.Fatalf("unexpected six payload/header: %d %v", six.Body.Len(), six.Header())
	}
}

func TestV2StemFramesRejectsUnknownOrOversizedRequestsAndCrossSongLookup(t *testing.T) {
	fixture := newStemFrameFixture(t, stems.LayoutFour)
	for _, tc := range []struct {
		url    string
		status int
	}{
		{"/song/set/frames?frameCount=1&layout=dj4&format=f32le", 400},
		{"/song/set/frames?startFrame=-1&frameCount=1&layout=dj4&format=f32le", 400},
		{"/song/set/frames?startFrame=0&frameCount=1&layout=nope&format=f32le", 400},
		{"/song/set/frames?startFrame=0&frameCount=1&layout=dj4&format=wav", 400},
		{"/song/set/frames?startFrame=0&frameCount=4&layout=dj4&format=f32le", 416},
		{"/other/set/frames?startFrame=0&frameCount=1&layout=dj4&format=f32le", 404},
		{"/song/not-registered/frames?startFrame=0&frameCount=1&layout=dj4&format=f32le", 404},
	} {
		rec := fixture.request(tc.url)
		if rec.Code != tc.status {
			t.Errorf("GET %s = %d, want %d; body %s", tc.url, rec.Code, tc.status, rec.Body.String())
		}
	}
	sets, err := fixture.database.ListStemSets("song")
	if err != nil {
		t.Fatal(err)
	}
	sets[0].Frames = 200000
	if err = fixture.database.UpsertStemSet(sets[0]); err != nil {
		t.Fatal(err)
	}
	large := fixture.request("/song/set/frames?startFrame=0&frameCount=200000&layout=dj4&format=f32le")
	if large.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized request status=%d, want 413", large.Code)
	}
}

func TestV2StemFramesRejectsChangedPackageAndSymlinkArtifacts(t *testing.T) {
	t.Run("changed checksum", func(t *testing.T) {
		fixture := newStemFrameFixture(t, stems.LayoutFour)
		first := fixture.request("/song/set/frames?startFrame=0&frameCount=1&layout=dj4&format=f32le")
		if first.Code != 200 {
			t.Fatalf("warm request status=%d", first.Code)
		}
		artifact := filepath.Join(fixture.packagePath, "vocals.wav")
		original, err := os.ReadFile(artifact)
		if err != nil {
			t.Fatal(err)
		}
		original[len(original)-1] ^= 1
		if err = os.WriteFile(artifact, original, 0600); err != nil {
			t.Fatal(err)
		}
		rec := fixture.request("/song/set/frames?startFrame=0&frameCount=1&layout=dj4&format=f32le")
		if rec.Code != http.StatusConflict {
			t.Fatalf("changed package status=%d, want 409", rec.Code)
		}
	})
	t.Run("manifest path traversal", func(t *testing.T) {
		fixture := newStemFrameFixture(t, stems.LayoutFour)
		manifestPath := filepath.Join(fixture.packagePath, stems.ManifestFilename)
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			t.Fatal(err)
		}
		changed := strings.Replace(string(data), `"path":"vocals.wav"`, `"path":"../outside.wav"`, 1)
		if changed == string(data) {
			t.Fatal("fixture did not contain expected artifact path")
		}
		if err = os.WriteFile(manifestPath, []byte(changed), 0600); err != nil {
			t.Fatal(err)
		}
		rec := fixture.request("/song/set/frames?startFrame=0&frameCount=1&layout=dj4&format=f32le")
		if rec.Code != http.StatusConflict {
			t.Fatalf("path traversal status=%d, want 409", rec.Code)
		}
	})
	t.Run("symlink artifact", func(t *testing.T) {
		fixture := newStemFrameFixture(t, stems.LayoutFour)
		outside := filepath.Join(t.TempDir(), "outside.wav")
		if err := os.WriteFile(outside, []byte("outside"), 0600); err != nil {
			t.Fatal(err)
		}
		artifact := filepath.Join(fixture.packagePath, "vocals.wav")
		_ = os.Remove(artifact)
		if err := os.Symlink(outside, artifact); err != nil {
			t.Skipf("symlink creation unavailable: %v", err)
		}
		rec := fixture.request("/song/set/frames?startFrame=0&frameCount=1&layout=dj4&format=f32le")
		if rec.Code != http.StatusConflict {
			t.Fatalf("symlink package status=%d, want 409", rec.Code)
		}
	})
	t.Run("symlink package root", func(t *testing.T) {
		fixture := newStemFrameFixture(t, stems.LayoutFour)
		alias := filepath.Join(t.TempDir(), "linked.viibstems")
		if err := os.Symlink(fixture.packagePath, alias); err != nil {
			t.Skipf("symlink creation unavailable: %v", err)
		}
		sets, err := fixture.database.ListStemSets("song")
		if err != nil {
			t.Fatal(err)
		}
		if err = fixture.database.DeleteStemSet("song", sets[0].ID); err != nil {
			t.Fatal(err)
		}
		sets[0].ID = "alias-set"
		sets[0].PackagePath = alias
		if err = fixture.database.UpsertStemSet(sets[0]); err != nil {
			t.Fatal(err)
		}
		rec := fixture.request("/song/alias-set/frames?startFrame=0&frameCount=1&layout=dj4&format=f32le")
		if rec.Code != http.StatusConflict {
			t.Fatalf("symlink root status=%d, want 409", rec.Code)
		}
	})
}

func TestV2StemFramesAcceptsRetaggedSourceWhenPCM32IdentityMatches(t *testing.T) {
	fixture := newStemFrameFixture(t, stems.LayoutFour)
	song, err := fixture.database.GetSongByID("song")
	if err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(song.FilePath)
	if err != nil {
		t.Fatal(err)
	}
	retagged := append([]byte(nil), original...)
	retagged = append(retagged, []byte("JUNK")...)
	retagged = append(retagged, 0, 0, 0, 0)
	binary.LittleEndian.PutUint32(retagged[4:8], uint32(len(retagged)-8))
	if err = os.WriteFile(song.FilePath, retagged, 0600); err != nil {
		t.Fatal(err)
	}
	rec := fixture.request("/song/set/frames?startFrame=0&frameCount=1&layout=dj4&format=f32le")
	if rec.Code != http.StatusOK {
		t.Fatalf("retag-equivalent source status=%d body=%s", rec.Code, rec.Body.String())
	}
}

type stemFrameAPIFixture struct {
	router      http.Handler
	packagePath string
	database    *db.DB
}

func (f stemFrameAPIFixture) request(url string) *httptest.ResponseRecorder {
	r := httptest.NewRecorder()
	f.router.ServeHTTP(r, httptest.NewRequest(http.MethodGet, url, nil))
	return r
}

func newStemFrameFixture(t *testing.T, layout stems.Layout) stemFrameAPIFixture {
	t.Helper()
	root := t.TempDir()
	source := filepath.Join(root, "source.wav")
	sourceBytes := frameTestWAV([]int16{0, 0, 1000, 1000, 2000, 2000})
	if err := os.WriteFile(source, sourceBytes, 0600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(sourceBytes)
	sourceHash := hex.EncodeToString(sum[:])
	audioHash, err := stemSourceAudioHashes.SHA256(context.Background(), decoderRegistry(), analysis.ResolvedSource{Name: filepath.Base(source), Path: source}, 48000, 2)
	if err != nil {
		t.Fatalf("compute fixture audio identity: %v", err)
	}
	packagePath := filepath.Join(root, "song.viibstems")
	if err := os.Mkdir(packagePath, 0700); err != nil {
		t.Fatal(err)
	}
	names := []stems.StemName{stems.StemVocals, stems.StemDrums, stems.StemBass, stems.StemOther}
	bases := map[stems.StemName]float32{stems.StemVocals: .10, stems.StemDrums: .20, stems.StemBass: .30, stems.StemOther: .40}
	if layout == stems.LayoutSix {
		names = []stems.StemName{stems.StemVocals, stems.StemDrums, stems.StemBass, stems.StemGuitar, stems.StemPiano, stems.StemOther}
		bases[stems.StemGuitar] = .04
		bases[stems.StemPiano] = .05
		bases[stems.StemOther] = .06
	}
	manifest := stems.Manifest{SchemaVersion: 1, Source: stems.Source{Filename: "source.wav", SHA256: sourceHash, AudioSHA256: audioHash, Duration: 3.0 / 48000}, StemLayout: layout, Generator: stems.Provenance{Name: "fixture", Version: "1"}, Model: stems.Provenance{Name: "fixture-model", Version: "1"}, Audio: stems.AudioGeometry{SampleRate: 48000, Channels: 2, Frames: 3}, Timing: stems.Timing{}, Stems: map[stems.StemName]stems.Artifact{}}
	for _, name := range names {
		samples := make([]int16, 6)
		for frame := 0; frame < 3; frame++ {
			for channel := 0; channel < 2; channel++ {
				value := bases[name] + float32(frame)*.01 + float32(channel)*.001
				samples[frame*2+channel] = int16(math.Round(float64(value * 32767)))
			}
		}
		data := frameTestWAV(samples)
		filename := string(name) + ".wav"
		path := filepath.Join(packagePath, filename)
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		hash := sha256.Sum256(data)
		manifest.Stems[name] = stems.Artifact{Path: filename, SHA256: hex.EncodeToString(hash[:]), SizeBytes: int64(len(data)), SampleRate: 48000, Channels: 2, Frames: 3, Encoding: "pcm_s16le"}
	}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(packagePath, stems.ManifestFilename), manifestBytes, 0600); err != nil {
		t.Fatal(err)
	}
	database, err := db.New(filepath.Join(root, "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err = database.SaveSong(&db.Song{ID: "song", Title: "Song", FilePath: source, AddedAt: 1}); err != nil {
		t.Fatal(err)
	}
	otherSource := filepath.Join(root, "other.wav")
	if err = os.WriteFile(otherSource, []byte("other source"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = database.SaveSong(&db.Song{ID: "other", Title: "Other", FilePath: otherSource, AddedAt: 1}); err != nil {
		t.Fatal(err)
	}
	set := db.StemSet{ID: "set", SongID: "song", SourceAudioHash: sourceHash, AudioSHA256: manifest.Source.AudioSHA256, ModelName: manifest.Model.Name, ModelVersion: manifest.Model.Version, GeneratorName: manifest.Generator.Name, GeneratorVersion: manifest.Generator.Version, Layout: string(layout), Status: "ready", SampleRate: 48000, Channels: 2, Frames: 3, DurationSeconds: 3.0 / 48000, PackagePath: packagePath, DiscoverySource: "explicit", ManifestSchemaVersion: 1, ExplicitlyLinked: true}
	for name, a := range manifest.Stems {
		set.Stems = append(set.Stems, db.StemArtifact{Name: string(name), RelativePath: a.Path, SHA256: a.SHA256, SizeBytes: a.SizeBytes, SampleRate: a.SampleRate, Channels: a.Channels, Frames: a.Frames, Encoding: a.Encoding})
	}
	if err = database.UpsertStemSet(set); err != nil {
		t.Fatal(err)
	}
	return stemFrameAPIFixture{router: (&API{db: database}).V2StemRoutes(), packagePath: packagePath, database: database}
}

func frameTestWAV(samples []int16) []byte {
	data := make([]byte, 44+len(samples)*2)
	copy(data[0:4], "RIFF")
	binary.LittleEndian.PutUint32(data[4:8], uint32(len(data)-8))
	copy(data[8:12], "WAVE")
	copy(data[12:16], "fmt ")
	binary.LittleEndian.PutUint32(data[16:20], 16)
	binary.LittleEndian.PutUint16(data[20:22], 1)
	binary.LittleEndian.PutUint16(data[22:24], 2)
	binary.LittleEndian.PutUint32(data[24:28], 48000)
	binary.LittleEndian.PutUint32(data[28:32], 48000*4)
	binary.LittleEndian.PutUint16(data[32:34], 4)
	binary.LittleEndian.PutUint16(data[34:36], 16)
	copy(data[36:40], "data")
	binary.LittleEndian.PutUint32(data[40:44], uint32(len(samples)*2))
	for i, s := range samples {
		binary.LittleEndian.PutUint16(data[44+i*2:], uint16(s))
	}
	return data
}
