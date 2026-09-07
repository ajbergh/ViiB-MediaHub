package analysisbench

import (
	"fmt"
	"math"
)

// NewSyncopatedClickTrack adds a weaker offbeat to every beat. Its known tempo
// remains the requested BPM, while the extra transient exposes double-tempo
// mistakes in onset/candidate implementations.
func NewSyncopatedClickTrack(name string, bpm, durationSeconds float64, sampleRate, channels int) (PCMFixture, error) {
	fixture, err := NewClickTrack(name, bpm, durationSeconds, sampleRate, channels)
	if err != nil {
		return PCMFixture{}, err
	}
	period := float64(sampleRate) * 60 / bpm
	for beat := 0; ; beat++ {
		start := int(math.Round((float64(beat) + 0.5) * period))
		if start >= fixture.Frames() {
			break
		}
		addPulse(fixture.Samples, start, sampleRate, channels, 0.45)
	}
	fixture.Kind = "syncopated-click-track"
	return fixture, nil
}

// NewMissingBeatClickTrack removes every nth beat from a known click track.
func NewMissingBeatClickTrack(name string, bpm, durationSeconds float64, sampleRate, channels, missingEvery int) (PCMFixture, error) {
	if missingEvery < 2 {
		return PCMFixture{}, fmt.Errorf("missingEvery must be at least 2")
	}
	if name == "" || bpm <= 0 || durationSeconds <= 0 || sampleRate <= 0 || channels <= 0 {
		return PCMFixture{}, fmt.Errorf("name, bpm, duration, sample rate, and channels must be positive")
	}
	frames := int(math.Round(durationSeconds * float64(sampleRate)))
	samples := make([]float32, frames*channels)
	period := float64(sampleRate) * 60 / bpm
	for beat := 0; ; beat++ {
		start := int(math.Round(float64(beat) * period))
		if start >= frames {
			break
		}
		if (beat+1)%missingEvery != 0 {
			addPulse(samples, start, sampleRate, channels, 1)
		}
	}
	return PCMFixture{
		Name: name, Kind: "missing-beat-click-track", SampleRate: sampleRate, Channels: channels,
		Samples: samples, Expected: ExpectedAnalysis{BPM: &bpm},
	}, nil
}

// NewTempoRampClickTrack creates a linearly changing tempo. It deliberately
// has no single expected BPM, allowing tests to assert that static analyzers do
// not present one as a reliable catalog fact.
func NewTempoRampClickTrack(name string, startBPM, endBPM, durationSeconds float64, sampleRate, channels int) (PCMFixture, error) {
	if name == "" || startBPM <= 0 || endBPM <= 0 || durationSeconds <= 0 || sampleRate <= 0 || channels <= 0 {
		return PCMFixture{}, fmt.Errorf("name, tempo endpoints, duration, sample rate, and channels must be positive")
	}
	frames := int(math.Round(durationSeconds * float64(sampleRate)))
	samples := make([]float32, frames*channels)
	for position := 0.0; position < float64(frames); {
		start := int(math.Round(position))
		addPulse(samples, start, sampleRate, channels, 1)
		progress := position / float64(frames)
		bpm := startBPM + (endBPM-startBPM)*progress
		position += float64(sampleRate) * 60 / bpm
	}
	return PCMFixture{
		Name: name, Kind: "tempo-ramp-click-track", SampleRate: sampleRate, Channels: channels,
		Samples: samples, Expected: ExpectedAnalysis{IsDynamic: true},
	}, nil
}

// NewTriad generates an equal-tempered additive major or minor triad for
// chroma/key invariants. tonic uses C=0 through B=11.
func NewTriad(name string, tonic int, minor bool, durationSeconds float64, sampleRate, channels int, tuningA4 float64) (PCMFixture, error) {
	if name == "" || tonic < 0 || tonic > 11 || durationSeconds <= 0 || sampleRate <= 0 || channels <= 0 || tuningA4 <= 0 {
		return PCMFixture{}, fmt.Errorf("name, tonic, duration, sample rate, channels, and tuning must be valid")
	}
	frames := int(math.Round(durationSeconds * float64(sampleRate)))
	samples := make([]float32, frames*channels)
	third := 4
	mode := "major"
	if minor {
		third = 3
		mode = "minor"
	}
	intervals := []int{0, third, 7}
	rootMIDI := 60 + tonic
	for frame := 0; frame < frames; frame++ {
		value := 0.0
		for _, interval := range intervals {
			frequency := tuningA4 * math.Pow(2, float64(rootMIDI+interval-69)/12)
			value += math.Sin(2*math.Pi*frequency*float64(frame)/float64(sampleRate)) / float64(len(intervals))
		}
		for channel := 0; channel < channels; channel++ {
			samples[frame*channels+channel] = float32(value * 0.45)
		}
	}
	return PCMFixture{
		Name: name, Kind: "additive-triad", SampleRate: sampleRate, Channels: channels, Samples: samples,
		Expected: ExpectedAnalysis{Key: tonicNames[tonic] + " " + mode},
	}, nil
}

// NewNoisyClickTrack adds deterministic pseudo-random noise to a click track;
// it does not use math/rand, keeping fixtures stable across Go versions.
func NewNoisyClickTrack(name string, bpm, durationSeconds float64, sampleRate, channels int, noiseAmplitude float32) (PCMFixture, error) {
	if noiseAmplitude < 0 || noiseAmplitude > 1 {
		return PCMFixture{}, fmt.Errorf("noise amplitude must be in [0, 1]")
	}
	fixture, err := NewClickTrack(name, bpm, durationSeconds, sampleRate, channels)
	if err != nil {
		return PCMFixture{}, err
	}
	state := uint32(0x5EED1234)
	for index := range fixture.Samples {
		state = state*1664525 + 1013904223
		noise := (float32(state>>8)/float32(1<<24))*2 - 1
		fixture.Samples[index] += noise * noiseAmplitude
	}
	fixture.Kind = "noisy-click-track"
	return fixture, nil
}

// NewQuietIntroClickTrack places a known-tempo click track after a silent
// intro, modeling the common case where a first-window analysis sees no beat.
func NewQuietIntroClickTrack(name string, bpm, durationSeconds, introSeconds float64, sampleRate, channels int) (PCMFixture, error) {
	if introSeconds <= 0 || introSeconds >= durationSeconds {
		return PCMFixture{}, fmt.Errorf("intro duration must be positive and shorter than the fixture")
	}
	body, err := NewClickTrack(name, bpm, durationSeconds-introSeconds, sampleRate, channels)
	if err != nil {
		return PCMFixture{}, err
	}
	introFrames := int(math.Round(introSeconds * float64(sampleRate)))
	samples := make([]float32, (introFrames+body.Frames())*channels)
	copy(samples[introFrames*channels:], body.Samples)
	body.Kind = "quiet-intro-click-track"
	body.Samples = samples
	return body, nil
}

// Phase0SyntheticFixtures covers the deterministic CI scenarios listed in
// roadmap §14.5. It is separate from DefaultFixtures so quick smoke commands
// remain fast and small.
func Phase0SyntheticFixtures() ([]PCMFixture, error) {
	fixtures, err := DefaultFixtures()
	if err != nil {
		return nil, err
	}
	add := func(fixture PCMFixture, fixtureErr error) error {
		if fixtureErr != nil {
			return fixtureErr
		}
		fixtures = append(fixtures, fixture)
		return nil
	}
	if err := add(NewSyncopatedClickTrack("syncopated-128", 128, 8, 44100, 2)); err != nil {
		return nil, err
	}
	if err := add(NewMissingBeatClickTrack("missing-every-fourth-124", 124, 8, 44100, 2, 4)); err != nil {
		return nil, err
	}
	if err := add(NewTempoRampClickTrack("tempo-ramp-110-130", 110, 130, 8, 44100, 2)); err != nil {
		return nil, err
	}
	if err := add(NewNoisyClickTrack("noisy-126", 126, 8, 44100, 2, 0.06)); err != nil {
		return nil, err
	}
	if err := add(NewQuietIntroClickTrack("quiet-intro-122", 122, 8, 2, 44100, 2)); err != nil {
		return nil, err
	}
	for tonic := 0; tonic < 12; tonic++ {
		if err := add(NewTriad("triad-"+tonicNames[tonic]+"-major", tonic, false, 1, 22050, 1, 440)); err != nil {
			return nil, err
		}
		if err := add(NewTriad("triad-"+tonicNames[tonic]+"-minor", tonic, true, 1, 22050, 1, 440)); err != nil {
			return nil, err
		}
	}
	if err := add(NewSineTone("detuned-a4-438", 438, 2, 44100, 2)); err != nil {
		return nil, err
	}
	if err := add(NewSineTone("detuned-a4-442", 442, 2, 44100, 2)); err != nil {
		return nil, err
	}
	return fixtures, nil
}

var tonicNames = [...]string{"C", "C#", "D", "D#", "E", "F", "F#", "G", "G#", "A", "A#", "B"}

func addPulse(samples []float32, start, sampleRate, channels int, amplitude float64) {
	if start < 0 || start >= len(samples)/channels {
		return
	}
	pulseFrames := max(1, int(math.Round(float64(sampleRate)*0.012)))
	for offset := 0; offset < pulseFrames && start+offset < len(samples)/channels; offset++ {
		envelope := math.Exp(-8 * float64(offset) / float64(pulseFrames))
		value := float32(amplitude * envelope * math.Sin(2*math.Pi*1000*float64(offset)/float64(sampleRate)))
		for channel := 0; channel < channels; channel++ {
			samples[(start+offset)*channels+channel] += value
		}
	}
}
