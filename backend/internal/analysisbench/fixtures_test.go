package analysisbench

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
)

func TestDefaultFixturesAreDeterministicAndCoverKnownUnknownCases(t *testing.T) {
	first, err := DefaultFixtures()
	if err != nil {
		t.Fatalf("DefaultFixtures() error = %v", err)
	}
	second, err := DefaultFixtures()
	if err != nil {
		t.Fatalf("DefaultFixtures() second call error = %v", err)
	}
	if len(first) != 4 || len(second) != len(first) {
		t.Fatalf("fixture count = %d and %d, want 4", len(first), len(second))
	}
	for index := range first {
		if first[index].Name != second[index].Name || !bytes.Equal(float32Bytes(t, first[index].Samples), float32Bytes(t, second[index].Samples)) {
			t.Fatalf("fixture %d is not deterministic", index)
		}
	}
	if first[1].Expected.BPM == nil || math.Abs(*first[1].Expected.BPM-128.5) > 1e-9 {
		t.Fatalf("fractional click BPM = %v, want 128.5", first[1].Expected.BPM)
	}
	if !first[3].Expected.IsUnknown {
		t.Fatal("silence must be marked unknown")
	}
}

func TestNewClickTrackRejectsInvalidParameters(t *testing.T) {
	if _, err := NewClickTrack("bad", 0, 1, 44100, 2); err == nil {
		t.Fatal("NewClickTrack accepted zero BPM")
	}
}

func TestPhase0SyntheticFixturesCoverRoadmapScenariosDeterministically(t *testing.T) {
	fixtures, err := Phase0SyntheticFixtures()
	if err != nil {
		t.Fatalf("Phase0SyntheticFixtures() error = %v", err)
	}
	if len(fixtures) != 35 {
		t.Fatalf("fixture count = %d, want 35", len(fixtures))
	}
	byName := make(map[string]PCMFixture, len(fixtures))
	triads := 0
	for _, fixture := range fixtures {
		byName[fixture.Name] = fixture
		if fixture.Kind == "additive-triad" {
			triads++
			if fixture.Expected.Key == "" {
				t.Fatalf("triad %q has no expected key", fixture.Name)
			}
		}
	}
	if triads != 24 {
		t.Fatalf("triad count = %d, want all 24 major/minor keys", triads)
	}
	if !byName["tempo-ramp-110-130"].Expected.IsDynamic || byName["tempo-ramp-110-130"].Expected.BPM != nil {
		t.Fatal("tempo ramp must not claim one static BPM")
	}
	quiet := byName["quiet-intro-122"]
	for _, sample := range quiet.Samples[:2*quiet.SampleRate*quiet.Channels] {
		if sample != 0 {
			t.Fatal("quiet intro contains non-silent PCM")
		}
	}
	noisyAgain, err := NewNoisyClickTrack("noisy-126", 126, 8, 44100, 2, 0.06)
	if err != nil {
		t.Fatalf("regenerate noisy fixture: %v", err)
	}
	if !bytes.Equal(float32Bytes(t, byName["noisy-126"].Samples), float32Bytes(t, noisyAgain.Samples)) {
		t.Fatal("noisy fixture is not deterministic")
	}
}

func TestExtendedFixtureConstructorsValidateArguments(t *testing.T) {
	if _, err := NewMissingBeatClickTrack("bad", 120, 1, 44100, 2, 1); err == nil {
		t.Fatal("NewMissingBeatClickTrack accepted missingEvery=1")
	}
	if _, err := NewTempoRampClickTrack("bad", 0, 120, 1, 44100, 2); err == nil {
		t.Fatal("NewTempoRampClickTrack accepted zero start BPM")
	}
	if _, err := NewTriad("bad", 12, false, 1, 44100, 2, 440); err == nil {
		t.Fatal("NewTriad accepted out-of-range tonic")
	}
}

func TestInspectWAVReadsPCMGeometryWithoutReadingPayload(t *testing.T) {
	payload := make([]byte, 16) // four stereo 16-bit frames
	wav := makeWAV(t, 1, 2, 44100, 16, payload)
	info, err := InspectWAV(bytes.NewReader(wav))
	if err != nil {
		t.Fatalf("InspectWAV() error = %v", err)
	}
	if info.Channels != 2 || info.SampleRate != 44100 || info.BitsPerSample != 16 || info.Frames != 4 {
		t.Fatalf("InspectWAV() = %+v, want stereo 44.1kHz 16-bit with 4 frames", info)
	}
}

func TestWriteWAVPCM16ProducesInspectableGeometry(t *testing.T) {
	fixture, err := NewClickTrack("fixture", 128, 1, 22050, 2)
	if err != nil {
		t.Fatalf("NewClickTrack() error = %v", err)
	}
	var encoded bytes.Buffer
	if err := WriteWAVPCM16(&encoded, fixture); err != nil {
		t.Fatalf("WriteWAVPCM16() error = %v", err)
	}
	info, err := InspectWAV(bytes.NewReader(encoded.Bytes()))
	if err != nil {
		t.Fatalf("InspectWAV() error = %v", err)
	}
	if info.AudioFormat != 1 || int(info.Channels) != fixture.Channels || int(info.SampleRate) != fixture.SampleRate || int(info.Frames) != fixture.Frames() {
		t.Fatalf("WAV info = %+v, want PCM fixture geometry", info)
	}
}

func TestWriteWAVPCM16ClampsSampleRange(t *testing.T) {
	fixture := PCMFixture{Name: "clamped", SampleRate: 44100, Channels: 1, Samples: []float32{-2, 2}}
	var encoded bytes.Buffer
	if err := WriteWAVPCM16(&encoded, fixture); err != nil {
		t.Fatalf("WriteWAVPCM16() error = %v", err)
	}
	payload := encoded.Bytes()[44:]
	if got := int16(binary.LittleEndian.Uint16(payload[:2])); got != math.MinInt16 {
		t.Fatalf("negative clipped sample = %d, want %d", got, math.MinInt16)
	}
	if got := int16(binary.LittleEndian.Uint16(payload[2:4])); got != math.MaxInt16 {
		t.Fatalf("positive clipped sample = %d, want %d", got, math.MaxInt16)
	}
}

func TestInspectWAVRejectsUnsupportedAndMisalignedData(t *testing.T) {
	unsupported := makeWAV(t, 6, 2, 44100, 16, make([]byte, 16))
	if _, err := InspectWAV(bytes.NewReader(unsupported)); err == nil {
		t.Fatal("InspectWAV accepted unsupported format code")
	}
	misaligned := makeWAV(t, 1, 2, 44100, 16, make([]byte, 15))
	if _, err := InspectWAV(bytes.NewReader(misaligned)); err == nil {
		t.Fatal("InspectWAV accepted data that is not frame-aligned")
	}
}

func TestCodecMatrixDoesNotClaimOpusIsVorbis(t *testing.T) {
	for _, codec := range CodecMatrix() {
		if codec.Format == "Opus in Ogg" && codec.Phase0State != "unsupported-pending-spike" {
			t.Fatalf("Opus Phase 0 state = %q, want unsupported-pending-spike", codec.Phase0State)
		}
	}
}

func makeWAV(t *testing.T, format, channels uint16, sampleRate uint32, bits uint16, payload []byte) []byte {
	t.Helper()
	var result bytes.Buffer
	result.WriteString("RIFF")
	if err := binary.Write(&result, binary.LittleEndian, uint32(4+8+16+8+len(payload))); err != nil {
		t.Fatal(err)
	}
	result.WriteString("WAVEfmt ")
	if err := binary.Write(&result, binary.LittleEndian, uint32(16)); err != nil {
		t.Fatal(err)
	}
	for _, value := range []any{format, channels, sampleRate, sampleRate * uint32(channels) * uint32(bits) / 8, uint16(channels * bits / 8), bits} {
		if err := binary.Write(&result, binary.LittleEndian, value); err != nil {
			t.Fatal(err)
		}
	}
	result.WriteString("data")
	if err := binary.Write(&result, binary.LittleEndian, uint32(len(payload))); err != nil {
		t.Fatal(err)
	}
	result.Write(payload)
	return result.Bytes()
}

func float32Bytes(t *testing.T, values []float32) []byte {
	t.Helper()
	result := make([]byte, len(values)*4)
	for index, value := range values {
		binary.LittleEndian.PutUint32(result[index*4:], math.Float32bits(value))
	}
	return result
}
