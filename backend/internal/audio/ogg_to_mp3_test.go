package audio

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type testPCMReader struct {
	samples    []float32
	position   int
	channels   int
	sampleRate int
}

func (r *testPCMReader) Read(buffer []float32) (int, error) {
	if r.position == len(r.samples) {
		return 0, io.EOF
	}
	n := copy(buffer, r.samples[r.position:])
	r.position += n
	if r.position == len(r.samples) {
		return n, io.EOF
	}
	return n, nil
}

func (r *testPCMReader) Channels() int   { return r.channels }
func (r *testPCMReader) SampleRate() int { return r.sampleRate }

func TestEncodeVorbisPCMProducesDecodableMP3(t *testing.T) {
	const sampleRate = 44100
	const channels = 2
	samples := make([]float32, sampleRate*channels/10)
	for i := 0; i < len(samples)/channels; i++ {
		sample := float32(math.Sin(2 * math.Pi * 440 * float64(i) / sampleRate))
		samples[i*channels] = sample
		samples[i*channels+1] = sample
	}

	reader := &testPCMReader{samples: samples, channels: channels, sampleRate: sampleRate}
	var encoded bytes.Buffer
	if err := writeID3v24(&encoded, MP3Metadata{Title: "Test track", Artist: "Test artist"}); err != nil {
		t.Fatalf("write tags: %v", err)
	}
	activityCalls := 0
	if err := encodeVorbisPCM(context.Background(), reader, &encoded, func() { activityCalls++ }); err != nil {
		t.Fatalf("encode PCM: %v", err)
	}
	if activityCalls == 0 {
		t.Fatal("activity callback was not called")
	}
	if bitrate := firstMP3BitrateKbps(t, encoded.Bytes()); bitrate != mp3BitrateKbps {
		t.Fatalf("generated MP3 bitrate = %d kbps, want %d kbps", bitrate, mp3BitrateKbps)
	}

	path := filepath.Join(t.TempDir(), "test.mp3")
	if err := os.WriteFile(path, encoded.Bytes(), 0600); err != nil {
		t.Fatalf("write MP3: %v", err)
	}
	if err := validateMP3(path); err != nil {
		t.Fatalf("generated MP3 is not decodable: %v", err)
	}
}

func TestEncodeVorbisPCMConvertsLongMonoFasterThanRealTime(t *testing.T) {
	const sampleRate = 44100
	const durationSeconds = 2
	samples := make([]float32, sampleRate*durationSeconds)
	for i := range samples {
		samples[i] = float32(math.Sin(2 * math.Pi * 440 * float64(i) / sampleRate))
	}

	reader := &testPCMReader{samples: samples, channels: 1, sampleRate: sampleRate}
	var encoded bytes.Buffer
	if err := writeID3v24(&encoded, MP3Metadata{Title: "Mono track"}); err != nil {
		t.Fatalf("write tags: %v", err)
	}
	started := time.Now()
	if err := encodeVorbisPCM(context.Background(), reader, &encoded, nil); err != nil {
		t.Fatalf("encode mono PCM: %v", err)
	}
	if elapsed := time.Since(started); elapsed > time.Duration(durationSeconds)*time.Second {
		t.Fatalf("encoding took %v for %ds of audio; expected faster than real time", elapsed, durationSeconds)
	}
	if bitrate := firstMP3BitrateKbps(t, encoded.Bytes()); bitrate != mp3BitrateKbps {
		t.Fatalf("generated MP3 bitrate = %d kbps, want %d kbps", bitrate, mp3BitrateKbps)
	}

	path := filepath.Join(t.TempDir(), "mono.mp3")
	if err := os.WriteFile(path, encoded.Bytes(), 0600); err != nil {
		t.Fatalf("write MP3: %v", err)
	}
	if err := validateMP3(path); err != nil {
		t.Fatalf("generated mono-source MP3 is not decodable: %v", err)
	}
}

func TestConvertOggToMP3FailureKeepsSourceAndCleansTemporaryFile(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "broken.ogg")
	if err := os.WriteFile(source, []byte("not an ogg file"), 0600); err != nil {
		t.Fatalf("write source: %v", err)
	}

	if _, err := ConvertOggToMP3(context.Background(), source, MP3Metadata{}, nil); err == nil {
		t.Fatal("invalid Ogg conversion unexpectedly succeeded")
	}
	if _, err := os.Stat(source); err != nil {
		t.Fatalf("source was not preserved after failure: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "broken.mp3")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("MP3 target exists after failed conversion: %v", err)
	}
	temps, err := filepath.Glob(filepath.Join(dir, ".viib-mp3-*.tmp"))
	if err != nil {
		t.Fatalf("inspect temporary files: %v", err)
	}
	if len(temps) != 0 {
		t.Fatalf("temporary files remain after failed conversion: %v", temps)
	}
}

func TestAvailableMP3TargetDoesNotOverwriteExistingFile(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "track.ogg")
	for _, name := range []string{"track.mp3", "track (1).mp3"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("existing"), 0600); err != nil {
			t.Fatalf("write existing target: %v", err)
		}
	}

	target, err := availableMP3Target(source)
	if err != nil {
		t.Fatalf("choose target: %v", err)
	}
	if want := filepath.Join(dir, "track (2).mp3"); target != want {
		t.Fatalf("target = %q, want %q", target, want)
	}
}

func TestWriteID3v24IncludesDownloadMetadata(t *testing.T) {
	metadata := MP3Metadata{
		Title:       "Title",
		Artist:      "Artist",
		Album:       "Album",
		AlbumArtist: "Album artist",
		TrackNumber: 7,
		DiscNumber:  2,
		Date:        "2026-09-04",
		Genre:       "Rock",
	}
	var encoded bytes.Buffer
	if err := writeID3v24(&encoded, metadata); err != nil {
		t.Fatalf("write ID3 tag: %v", err)
	}

	data := encoded.Bytes()
	if len(data) < 10 || string(data[:3]) != "ID3" || data[3] != 4 {
		t.Fatalf("invalid ID3v2.4 header: %v", data)
	}
	frames := parseTextFrames(t, data[10:])
	want := map[string]string{
		"TIT2": "Title", "TPE1": "Artist", "TALB": "Album", "TPE2": "Album artist",
		"TRCK": "7", "TPOS": "2", "TDRC": "2026-09-04", "TCON": "Rock",
	}
	for id, value := range want {
		if frames[id] != value {
			t.Errorf("frame %s = %q, want %q", id, frames[id], value)
		}
	}
}

func TestEncodeVorbisPCMHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	reader := &testPCMReader{samples: []float32{0, 0}, channels: 2, sampleRate: 44100}
	if err := encodeVorbisPCM(ctx, reader, io.Discard, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled encoding returned %v", err)
	}
}

func parseTextFrames(t *testing.T, data []byte) map[string]string {
	t.Helper()
	frames := make(map[string]string)
	for len(data) >= 10 {
		id := string(data[:4])
		if strings.Trim(id, "\x00") == "" {
			break
		}
		size := int(data[4])<<21 | int(data[5])<<14 | int(data[6])<<7 | int(data[7])
		if size < 1 || 10+size > len(data) {
			t.Fatalf("invalid %s frame size %d", id, size)
		}
		if binary.BigEndian.Uint16(data[8:10]) != 0 {
			t.Fatalf("unexpected %s frame flags", id)
		}
		if data[10] != 3 {
			t.Fatalf("unexpected %s text encoding %d", id, data[10])
		}
		frames[id] = string(data[11 : 10+size])
		data = data[10+size:]
	}
	return frames
}

func firstMP3BitrateKbps(t *testing.T, data []byte) int {
	t.Helper()
	if len(data) < 10 || string(data[:3]) != "ID3" {
		t.Fatal("generated audio is missing its ID3 header")
	}
	tagSize := int(data[6])<<21 | int(data[7])<<14 | int(data[8])<<7 | int(data[9])
	frameOffset := 10 + tagSize
	if frameOffset+4 > len(data) {
		t.Fatal("generated audio is missing an MP3 frame")
	}
	header := binary.BigEndian.Uint32(data[frameOffset : frameOffset+4])
	if header>>21 != 0x7ff {
		t.Fatalf("invalid MP3 frame sync: %#x", header)
	}
	if version := (header >> 19) & 0x3; version != 0x3 {
		t.Fatalf("MP3 version bits = %d, want MPEG-1", version)
	}
	if layer := (header >> 17) & 0x3; layer != 0x1 {
		t.Fatalf("MP3 layer bits = %d, want Layer III", layer)
	}
	bitrates := [...]int{0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320}
	index := int((header >> 12) & 0xf)
	if index <= 0 || index >= len(bitrates) {
		t.Fatalf("invalid MPEG-1 Layer III bitrate index %d", index)
	}
	return bitrates[index]
}
