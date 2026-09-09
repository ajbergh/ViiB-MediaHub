// Package beatgrid defines the durable beat-grid contract used by analysis,
// persistence, and DJ playback. It intentionally owns no decoder or database
// dependency so every layer shares the same validated representation.
package beatgrid

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
)

const (
	// ArtifactKind, FormatVersion, and Encoding identify the representation in
	// track_analysis_artifacts. Changing byte semantics requires a new format
	// version; callers must never guess how to decode an old grid.
	ArtifactKind  = "beatgrid"
	FormatVersion = 1
	Encoding      = "beatgrid-v1-varint-us"
	maxBeats      = 200_000
)

var artifactMagic = [4]byte{'V', 'B', 'G', 1}

// Grid is a time-ordered beat timeline in seconds. Downbeats are indices into
// Beats, rather than duplicate timestamps, so a caller can retain arbitrary
// meter and later support irregular/downbeat-only annotations.
type Grid struct {
	Beats           []float64
	DownbeatIndices []int
}

// BuildStraight constructs a constant-tempo grid beginning at firstDownbeat.
// It is the Phase 6 bridge for existing static BPM results. A future phase
// tracker will supply an audio-derived firstDownbeat instead of zero; neither
// persistence nor playback needs to change when it does.
func BuildStraight(bpm, firstDownbeat, duration float64, beatsPerBar int) (Grid, error) {
	if !finitePositive(bpm) {
		return Grid{}, errors.New("beatgrid BPM must be finite and positive")
	}
	if !finiteNonNegative(firstDownbeat) || !finitePositive(duration) || firstDownbeat >= duration {
		return Grid{}, errors.New("beatgrid timing must be finite and within track duration")
	}
	if beatsPerBar < 1 || beatsPerBar > 32 {
		return Grid{}, errors.New("beatgrid beats per bar must be between 1 and 32")
	}
	interval := 60 / bpm
	grid := Grid{Beats: make([]float64, 0, int((duration-firstDownbeat)/interval)+1)}
	for beat := firstDownbeat; beat < duration; beat += interval {
		grid.Beats = append(grid.Beats, beat)
		if (len(grid.Beats)-1)%beatsPerBar == 0 {
			grid.DownbeatIndices = append(grid.DownbeatIndices, len(grid.Beats)-1)
		}
		if len(grid.Beats) > maxBeats {
			return Grid{}, errors.New("beatgrid exceeds maximum supported beat count")
		}
	}
	return grid, grid.Validate()
}

// Validate rejects malformed grids before they reach rendering or Sync. A
// strict ascending order makes the current-beat search unambiguous.
func (g Grid) Validate() error {
	if len(g.Beats) == 0 {
		return errors.New("beatgrid requires at least one beat")
	}
	if len(g.Beats) > maxBeats {
		return errors.New("beatgrid exceeds maximum supported beat count")
	}
	previous := -1.0
	for index, beat := range g.Beats {
		if !finiteNonNegative(beat) || (index > 0 && beat <= previous) {
			return fmt.Errorf("beatgrid beat %d is not a finite ascending timestamp", index)
		}
		previous = beat
	}
	previousIndex := -1
	for _, index := range g.DownbeatIndices {
		if index < 0 || index >= len(g.Beats) || index <= previousIndex {
			return errors.New("beatgrid downbeat indices must be ordered beat indices")
		}
		previousIndex = index
	}
	return nil
}

// Encode stores timestamps as non-negative microsecond deltas followed by
// delta-coded downbeat indices. The representation is compact, deterministic,
// and preserves sub-millisecond alignment needed by DJ beat phase operations.
func (g Grid) Encode() ([]byte, error) {
	if err := g.Validate(); err != nil {
		return nil, err
	}
	var output bytes.Buffer
	if _, err := output.Write(artifactMagic[:]); err != nil {
		return nil, err
	}
	writeUvarint(&output, uint64(len(g.Beats)))
	previousMicros := uint64(0)
	for index, beat := range g.Beats {
		micros := uint64(math.Round(beat * 1_000_000))
		if index > 0 && micros <= previousMicros {
			return nil, errors.New("beatgrid timestamps collapse at microsecond precision")
		}
		writeUvarint(&output, micros-previousMicros)
		previousMicros = micros
	}
	writeUvarint(&output, uint64(len(g.DownbeatIndices)))
	previousIndex := -1
	for _, index := range g.DownbeatIndices {
		writeUvarint(&output, uint64(index-previousIndex-1))
		previousIndex = index
	}
	return output.Bytes(), nil
}

// Decode validates and reconstructs a VBG v1 artifact. Unknown or truncated
// formats fail closed: a caller can fall back to BPM-only behavior instead of
// silently aligning playback to corrupt timing data.
func Decode(data []byte) (Grid, error) {
	if len(data) < len(artifactMagic) || !bytes.Equal(data[:len(artifactMagic)], artifactMagic[:]) {
		return Grid{}, errors.New("unsupported beatgrid artifact format")
	}
	reader := bytes.NewReader(data[len(artifactMagic):])
	beatCount, err := binary.ReadUvarint(reader)
	if err != nil || beatCount == 0 || beatCount > maxBeats {
		return Grid{}, errors.New("invalid beatgrid beat count")
	}
	grid := Grid{Beats: make([]float64, 0, beatCount)}
	previousMicros := uint64(0)
	for index := uint64(0); index < beatCount; index++ {
		delta, err := binary.ReadUvarint(reader)
		if err != nil || (index > 0 && delta == 0) || ^uint64(0)-previousMicros < delta {
			return Grid{}, errors.New("invalid beatgrid timestamp delta")
		}
		previousMicros += delta
		grid.Beats = append(grid.Beats, float64(previousMicros)/1_000_000)
	}
	downbeatCount, err := binary.ReadUvarint(reader)
	if err != nil || downbeatCount > beatCount {
		return Grid{}, errors.New("invalid beatgrid downbeat count")
	}
	previousIndex := -1
	for index := uint64(0); index < downbeatCount; index++ {
		delta, err := binary.ReadUvarint(reader)
		if err != nil || delta > uint64(len(grid.Beats)) {
			return Grid{}, errors.New("invalid beatgrid downbeat delta")
		}
		downbeat := previousIndex + int(delta) + 1
		if downbeat >= len(grid.Beats) {
			return Grid{}, errors.New("beatgrid downbeat index out of range")
		}
		grid.DownbeatIndices = append(grid.DownbeatIndices, downbeat)
		previousIndex = downbeat
	}
	if reader.Len() != 0 {
		return Grid{}, errors.New("unexpected trailing beatgrid artifact data")
	}
	return grid, grid.Validate()
}

func writeUvarint(output *bytes.Buffer, value uint64) {
	var encoded [binary.MaxVarintLen64]byte
	written := binary.PutUvarint(encoded[:], value)
	_, _ = output.Write(encoded[:written])
}

func finitePositive(value float64) bool {
	return value > 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

func finiteNonNegative(value float64) bool {
	return value >= 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}
