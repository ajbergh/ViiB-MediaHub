// Tests the canonical media capability matrix, reusable capability errors, and
// metadata extraction across supported and unsupported container fixtures.
package audio

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func buildValidWAVFixture(sampleRate, channels, frames int) []byte {
	var buf bytes.Buffer
	bytesPerSample := 2
	dataSize := uint32(frames * channels * bytesPerSample)
	riffSize := 36 + dataSize

	buf.WriteString("RIFF")
	_ = binary.Write(&buf, binary.LittleEndian, riffSize)
	buf.WriteString("WAVE")
	buf.WriteString("fmt ")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(16))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1)) // PCM
	_ = binary.Write(&buf, binary.LittleEndian, uint16(channels))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(sampleRate))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(sampleRate*channels*bytesPerSample))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(channels*bytesPerSample))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(16))
	buf.WriteString("data")
	_ = binary.Write(&buf, binary.LittleEndian, dataSize)
	buf.Write(make([]byte, int(dataSize)))
	return buf.Bytes()
}

func buildID3v2MP3Fixture(title, artist, album string) []byte {
	makeTextFrame := func(id, value string) []byte {
		payload := append([]byte{0x03}, []byte(value)...) // UTF-8
		var frame bytes.Buffer
		frame.WriteString(id)
		size := len(payload)
		frame.Write([]byte{
			byte((size >> 21) & 0x7f),
			byte((size >> 14) & 0x7f),
			byte((size >> 7) & 0x7f),
			byte(size & 0x7f),
		})
		frame.Write([]byte{0x00, 0x00})
		frame.Write(payload)
		return frame.Bytes()
	}

	var frames bytes.Buffer
	frames.Write(makeTextFrame("TIT2", title))
	frames.Write(makeTextFrame("TPE1", artist))
	frames.Write(makeTextFrame("TALB", album))

	var tag bytes.Buffer
	tag.WriteString("ID3")
	tag.Write([]byte{0x04, 0x00, 0x00}) // v2.4
	size := frames.Len()
	tag.Write([]byte{
		byte((size >> 21) & 0x7f),
		byte((size >> 14) & 0x7f),
		byte((size >> 7) & 0x7f),
		byte(size & 0x7f),
	})
	tag.Write(frames.Bytes())
	// Append a minimal MPEG1 Layer3 128kbps 44.1kHz frame
	frame := make([]byte, 417)
	frame[0] = 0xFF
	frame[1] = 0xFB
	frame[2] = 0x90
	frame[3] = 0x64
	tag.Write(frame)
	return tag.Bytes()
}

func buildFLACVorbisCommentFixture(title, artist, album string) []byte {
	var comment bytes.Buffer
	writeStr := func(s string) {
		_ = binary.Write(&comment, binary.LittleEndian, uint32(len(s)))
		comment.WriteString(s)
	}
	writeStr("viib-test")
	_ = binary.Write(&comment, binary.LittleEndian, uint32(3))
	writeStr("TITLE=" + title)
	writeStr("ARTIST=" + artist)
	writeStr("ALBUM=" + album)

	var out bytes.Buffer
	out.WriteString("fLaC")
	// STREAMINFO block (type 0, length 34)
	out.Write([]byte{0x00, 0x00, 0x00, 34})
	streamInfo := make([]byte, 34)
	// 44100 Hz, 2 channels, 16 bps, 44100 total samples
	streamInfo[10] = 0x0A
	streamInfo[11] = 0xC4
	streamInfo[12] = 0x42
	streamInfo[13] = 0xF0
	streamInfo[16] = 0xAC
	streamInfo[17] = 0x44
	out.Write(streamInfo)
	// VORBIS_COMMENT block (last block = 0x80 | 4 = 0x84)
	vcBytes := comment.Bytes()
	out.Write([]byte{0x84, byte(len(vcBytes) >> 16), byte(len(vcBytes) >> 8), byte(len(vcBytes))})
	out.Write(vcBytes)
	return out.Bytes()
}

func buildMP4iTunesM4AFixture(title, artist, album string, trackNum, discNum, year int) []byte {
	box := func(kind string, payload []byte) []byte {
		var b bytes.Buffer
		_ = binary.Write(&b, binary.BigEndian, uint32(8+len(payload)))
		b.WriteString(kind[:4])
		b.Write(payload)
		return b.Bytes()
	}
	textAtom := func(atomType, value string) []byte {
		var dataPayload bytes.Buffer
		_ = binary.Write(&dataPayload, binary.BigEndian, uint32(1)) // UTF-8 text
		_ = binary.Write(&dataPayload, binary.BigEndian, uint32(0))
		dataPayload.WriteString(value)
		return box(atomType, box("data", dataPayload.Bytes()))
	}
	pairAtom := func(atomType string, index, total uint16) []byte {
		var dataPayload bytes.Buffer
		_ = binary.Write(&dataPayload, binary.BigEndian, uint32(0)) // binary implicit
		_ = binary.Write(&dataPayload, binary.BigEndian, uint32(0))
		_ = binary.Write(&dataPayload, binary.BigEndian, uint16(0))
		_ = binary.Write(&dataPayload, binary.BigEndian, index)
		_ = binary.Write(&dataPayload, binary.BigEndian, total)
		_ = binary.Write(&dataPayload, binary.BigEndian, uint16(0))
		return box(atomType, box("data", dataPayload.Bytes()))
	}

	var ilstPayload bytes.Buffer
	ilstPayload.Write(textAtom("\xa9nam", title))
	ilstPayload.Write(textAtom("\xa9ART", artist))
	ilstPayload.Write(textAtom("\xa9alb", album))
	if year > 0 {
		ilstPayload.Write(textAtom("\xa9day", "2024"))
	}
	if trackNum > 0 {
		ilstPayload.Write(pairAtom("trkn", uint16(trackNum), 12))
	}
	if discNum > 0 {
		ilstPayload.Write(pairAtom("disk", uint16(discNum), 2))
	}

	metaPayload := append([]byte{0, 0, 0, 0}, box("ilst", ilstPayload.Bytes())...)
	udta := box("udta", box("meta", metaPayload))
	moov := box("moov", udta)

	var ftypPayload bytes.Buffer
	ftypPayload.WriteString("M4A ")
	_ = binary.Write(&ftypPayload, binary.BigEndian, uint32(0))
	ftypPayload.WriteString("M4A mp42isom")
	ftyp := box("ftyp", ftypPayload.Bytes())

	return append(ftyp, moov...)
}

func TestDefaultCapabilityMatrixCompletenessAndConsistency(t *testing.T) {
	matrix := DefaultCapabilityMatrix()
	if matrix.Version != CapabilityMatrixVersion {
		t.Fatalf("matrix version = %q, want %q", matrix.Version, CapabilityMatrixVersion)
	}

	requiredIDs := map[string]bool{
		"mp3":        false,
		"ogg_vorbis": false,
		"wav":        false,
		"flac":       false,
		"m4a_aac":    false,
		"opus":       false,
		"wma":        false,
	}

	for _, f := range matrix.Formats {
		if _, exists := requiredIDs[f.ID]; exists {
			requiredIDs[f.ID] = true
		}
		if len(f.Extensions) == 0 || len(f.MIMETypes) == 0 {
			t.Fatalf("format %s missing extensions or MIME types", f.ID)
		}
		if _, hasLocal := f.Sources["local"]; !hasLocal {
			t.Fatalf("format %s missing local source capability", f.ID)
		}
		if _, hasBrowser := f.Sources["browser"]; !hasBrowser {
			t.Fatalf("format %s missing browser source capability", f.ID)
		}
		if _, hasPlex := f.Sources["plex"]; !hasPlex {
			t.Fatalf("format %s missing plex source capability", f.ID)
		}
	}

	for id, seen := range requiredIDs {
		if !seen {
			t.Fatalf("canonical matrix missing required format %q", id)
		}
	}

	scannerExts := ScannerSupportedExtensions()
	for _, ext := range []string{".mp3", ".ogg", ".oga", ".wav", ".wave", ".flac", ".m4a", ".aac", ".opus", ".wma"} {
		if !scannerExts[ext] {
			t.Fatalf("expected scanner extension %s to be supported", ext)
		}
	}
}

func TestLookupFormatCapabilityAndErrors(t *testing.T) {
	flacCap, ok := LookupFormatCapability("Album/01 - Track.FLAC")
	if !ok || flacCap.ID != "flac" {
		t.Fatalf("LookupFormatCapability(FLAC) = %+v, %v", flacCap, ok)
	}
	if flacCap.BackendAnalysis != CapabilityUnsupported || flacCap.BackendWaveform != CapabilityClientFallback {
		t.Fatalf("unexpected FLAC backend capability state: %+v", flacCap)
	}

	waveformErr := NewWaveformCapabilityError("song.m4a")
	if waveformErr.Code != CapabilityErrorClientWaveformFallback || !waveformErr.FallbackAvailable || waveformErr.FormatID != "m4a_aac" {
		t.Fatalf("unexpected waveform capability error for m4a: %+v", waveformErr)
	}

	analysisErr := NewAnalysisCapabilityError("song.opus")
	if analysisErr.Code != CapabilityErrorAnalysisUnsupported || analysisErr.FormatID != "opus" || analysisErr.SupportState != CapabilityUnsupported {
		t.Fatalf("unexpected analysis capability error for opus: %+v", analysisErr)
	}
}

func TestExtractMetadataFixturesAcrossFormats(t *testing.T) {
	dir := t.TempDir()

	// 1. MP3 with ID3v2 tags
	mp3Path := filepath.Join(dir, "fixture.mp3")
	if err := os.WriteFile(mp3Path, buildID3v2MP3Fixture("Test MP3 Title", "Test MP3 Artist", "Test MP3 Album"), 0600); err != nil {
		t.Fatal(err)
	}
	mp3Meta, err := ExtractMetadata(mp3Path)
	if err != nil {
		t.Fatalf("ExtractMetadata(mp3): %v", err)
	}
	if mp3Meta.Title != "Test MP3 Title" || mp3Meta.Artist != "Test MP3 Artist" || mp3Meta.Album != "Test MP3 Album" {
		t.Fatalf("unexpected MP3 metadata: %+v", mp3Meta)
	}

	// 2. OGG Vorbis real fixture from repository testdata
	oggFixture := filepath.Join("..", "spotify", "testdata", "metadata.ogg")
	if _, statErr := os.Stat(oggFixture); statErr == nil {
		oggMeta, err := ExtractMetadata(oggFixture)
		if err != nil {
			t.Fatalf("ExtractMetadata(ogg): %v", err)
		}
		if oggMeta.ID == "" || oggMeta.Title == "" {
			t.Fatalf("unexpected OGG metadata: %+v", oggMeta)
		}
	}

	// 3. FLAC with Vorbis comments
	flacPath := filepath.Join(dir, "fixture.flac")
	if err := os.WriteFile(flacPath, buildFLACVorbisCommentFixture("Test FLAC Title", "Test FLAC Artist", "Test FLAC Album"), 0600); err != nil {
		t.Fatal(err)
	}
	flacMeta, err := ExtractMetadata(flacPath)
	if err != nil {
		t.Fatalf("ExtractMetadata(flac): %v", err)
	}
	if flacMeta.Title != "Test FLAC Title" || flacMeta.Artist != "Test FLAC Artist" || flacMeta.Album != "Test FLAC Album" {
		t.Fatalf("unexpected FLAC metadata: %+v", flacMeta)
	}

	// 4. WAV PCM16 fixture
	wavPath := filepath.Join(dir, "fixture_tone.wav")
	if err := os.WriteFile(wavPath, buildValidWAVFixture(44100, 2, 44100), 0600); err != nil {
		t.Fatal(err)
	}
	wavMeta, err := ExtractMetadata(wavPath)
	if err != nil {
		t.Fatalf("ExtractMetadata(wav): %v", err)
	}
	if wavMeta.Title != "fixture_tone" || wavMeta.Artist == "" || wavMeta.Album == "" {
		t.Fatalf("unexpected WAV fallback metadata: %+v", wavMeta)
	}

	// 5. Real MP4/M4A container fixture with iTunes ilst atoms
	m4aTaggedPath := filepath.Join(dir, "fixture_tagged.m4a")
	if err := os.WriteFile(m4aTaggedPath, buildMP4iTunesM4AFixture("Test M4A Title", "Test M4A Artist", "Test M4A Album", 5, 2, 2024), 0600); err != nil {
		t.Fatal(err)
	}
	m4aTaggedMeta, err := ExtractMetadata(m4aTaggedPath)
	if err != nil {
		t.Fatalf("ExtractMetadata(m4a tagged): %v", err)
	}
	if m4aTaggedMeta.Title != "Test M4A Title" || m4aTaggedMeta.Artist != "Test M4A Artist" || m4aTaggedMeta.Album != "Test M4A Album" || m4aTaggedMeta.TrackNumber != 5 || m4aTaggedMeta.DiscNumber != 2 || m4aTaggedMeta.Year != 2024 {
		t.Fatalf("unexpected M4A iTunes metadata: %+v", m4aTaggedMeta)
	}

	// 6. Minimal untagged/corrupt container fallback fixtures ensure ExtractMetadata
	// never panics and produces deterministic IDs and fallback fields.
	for _, ext := range []string{".m4a", ".aac", ".opus", ".wma"} {
		p := filepath.Join(dir, "sample"+ext)
		if err := os.WriteFile(p, []byte("minimal-container-bytes"), 0600); err != nil {
			t.Fatal(err)
		}
		meta, err := ExtractMetadata(p)
		if err != nil {
			t.Fatalf("ExtractMetadata(%s): %v", ext, err)
		}
		if meta.ID == "" || meta.Title != "sample" || meta.Artist != "Unknown Artist" || meta.Album != "Unknown Album" {
			t.Fatalf("unexpected fallback metadata for %s: %+v", ext, meta)
		}
	}
}
