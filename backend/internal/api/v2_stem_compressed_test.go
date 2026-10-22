package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/stems"
)

func TestV2CompressedStemPreviewAndFramePlayback(t *testing.T) {
	for _, format := range []string{"mp3", "ogg"} {
		t.Run(format, func(t *testing.T) {
			f := newStemFrameFixture(t, stems.LayoutSix)
			shared := filepath.Join("..", "..", "..", "fixtures", "stems")
			path := filepath.Join(shared, "package-v1-"+format+"-stereo-ffmpeg.viibstems")
			v, err := stems.ValidatePackage(path)
			if err != nil {
				t.Fatal(err)
			}
			m := v.Manifest
			for name, a := range m.Stems {
				b, err := os.ReadFile(v.Files[name])
				if err != nil {
					t.Fatal(err)
				}
				if err = os.WriteFile(filepath.Join(f.packagePath, a.Path), b, 0600); err != nil {
					t.Fatal(err)
				}
			}
			b, _ := json.Marshal(m)
			if err = os.WriteFile(filepath.Join(f.packagePath, stems.ManifestFilename), b, 0600); err != nil {
				t.Fatal(err)
			}
			song, err := f.database.GetSongByID("song")
			if err != nil {
				t.Fatal(err)
			}
			song.FilePath, err = filepath.Abs(filepath.Join(shared, "source", m.Source.Filename))
			if err != nil {
				t.Fatal(err)
			}
			if err = f.database.SaveSong(song); err != nil {
				t.Fatal(err)
			}
			sets, err := f.database.ListStemSets("song")
			if err != nil {
				t.Fatal(err)
			}
			set := sets[0]
			set.SourceAudioHash = m.Source.SHA256
			set.AudioSHA256 = m.Source.AudioSHA256
			set.SampleRate = m.Audio.SampleRate
			set.Channels = m.Audio.Channels
			set.Frames = m.Audio.Frames
			set.DurationSeconds = m.Source.Duration
			set.Stems = nil
			for name, a := range m.Stems {
				set.Stems = append(set.Stems, db.StemArtifact{Name: string(name), RelativePath: a.Path, SHA256: a.SHA256, SizeBytes: a.SizeBytes, SampleRate: a.SampleRate, Channels: a.Channels, Frames: a.Frames, Encoding: a.Encoding})
			}
			if err = f.database.UpsertStemSet(set); err != nil {
				t.Fatal(err)
			}
			response := f.request("/song/set/frames?startFrame=400&frameCount=128&layout=dj4&format=f32le")
			if response.Code != http.StatusOK || response.Body.Len() != 128*4*2*4 {
				t.Fatalf("frame response %d: %s", response.Code, response.Body.String())
			}
			req := httptest.NewRequest(http.MethodGet, "/song/set/vocals", nil)
			req.Header.Set("Range", "bytes=0-31")
			preview := httptest.NewRecorder()
			f.router.ServeHTTP(preview, req)
			mime := "audio/mpeg"
			if format == "ogg" {
				mime = "audio/ogg"
			}
			if preview.Code != http.StatusPartialContent || preview.Header().Get("Content-Type") != mime || preview.Body.Len() != 32 {
				t.Fatalf("preview status=%d headers=%v body=%s", preview.Code, preview.Header(), preview.Body.String())
			}
			// A changed encoded payload cannot reuse registry readiness or cached validation.
			a := m.Stems[stems.StemVocals]
			artifact := filepath.Join(f.packagePath, a.Path)
			bytes, err := os.ReadFile(artifact)
			if err != nil {
				t.Fatal(err)
			}
			bytes[len(bytes)/2] ^= 1
			if err = os.WriteFile(artifact, bytes, 0600); err != nil {
				t.Fatal(err)
			}
			if changed := f.request("/song/set/frames?startFrame=0&frameCount=1&layout=dj4&format=f32le"); changed.Code != http.StatusConflict {
				t.Fatalf("tampered frames status=%d", changed.Code)
			}
			if changed := f.request("/song/set/vocals"); changed.Code != http.StatusConflict {
				t.Fatalf("tampered preview status=%d", changed.Code)
			}
		})
	}
}

func TestCompressedRequestsStopValidationAndSnapshotWhenCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := validatedStemPackageContext(ctx, "missing"); !errors.Is(err, context.Canceled) {
		t.Fatalf("validation error %v", err)
	}
	if _, _, err := snapshotStemArtifactContext(ctx, nil, "missing", stems.Artifact{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("snapshot error %v", err)
	}
}
