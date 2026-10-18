package features

import (
	"math"
	"sort"
)

const EventVersion = "native-novelty-v1"

// StructureEvent describes observed change, not an inferred chorus/drop or
// calibrated probability. Position is deliberately unsnapped: grids can be edited
// after analysis. Alignment belongs to cue generation, not the cached evidence.
type StructureEvent struct {
	Position     float64 `json:"position"`
	Kind         string  `json:"kind"`
	Strength     float64 `json:"strength"`
	EnergyBefore float64 `json:"energyBefore"`
	EnergyAfter  float64 `json:"energyAfter"`
	TimbreChange float64 `json:"timbreChange"`
}

// Keep 20 compact summaries/second, never PCM. Two one-pole crossovers measure
// broad changes of arrangement even when full-band RMS stays constant. This is
// not source separation or vocal/drum classification.
type noveltyFrame struct{ power, low, mid, high float64 }
type noveltyAccumulator struct {
	window, filled                                 int
	step, lowAlpha, highAlpha, lowState, highState float64
	sum                                            noveltyFrame
	frames                                         []noveltyFrame
}

func newNoveltyAccumulator(rate int) *noveltyAccumulator {
	window := max(1, rate/20)
	return &noveltyAccumulator{window: window, step: float64(window) / math.Max(1, float64(rate)),
		lowAlpha:  1 - math.Exp(-2*math.Pi*200/math.Max(1, float64(rate))),
		highAlpha: 1 - math.Exp(-2*math.Pi*2500/math.Max(1, float64(rate)))}
}

func (a *noveltyAccumulator) feed(samples []float32) {
	for _, sample := range samples {
		x := float64(sample)
		a.lowState += a.lowAlpha * (x - a.lowState)
		a.highState += a.highAlpha * (x - a.highState)
		mid, high := a.highState-a.lowState, x-a.highState
		a.sum.power += x * x
		a.sum.low += a.lowState * a.lowState
		a.sum.mid += mid * mid
		a.sum.high += high * high
		a.filled++
		if a.filled == a.window {
			a.frames = append(a.frames, a.sum.scale(1/float64(a.filled)))
			a.sum, a.filled = noveltyFrame{}, 0
		}
	}
}

func (f noveltyFrame) scale(v float64) noveltyFrame {
	return noveltyFrame{f.power * v, f.low * v, f.mid * v, f.high * v}
}
func (f noveltyFrame) add(g noveltyFrame) noveltyFrame {
	return noveltyFrame{f.power + g.power, f.low + g.low, f.mid + g.mid, f.high + g.high}
}
func (f noveltyFrame) sub(g noveltyFrame) noveltyFrame {
	return noveltyFrame{math.Max(0, f.power-g.power), math.Max(0, f.low-g.low), math.Max(0, f.mid-g.mid), math.Max(0, f.high-g.high)}
}

func contrast(before, after noveltyFrame, floor float64) (float64, float64) {
	if math.Max(before.power, after.power) < floor {
		return 0, 0
	}
	// Keep the unsaturated contrast until peak picking. Clipping here makes a
	// large jump a plateau and incorrectly chooses a point before its arrival.
	energy := math.Abs(math.Log((after.power+floor)/(before.power+floor))) / math.Log(16)
	bs, as := before.low+before.mid+before.high, after.low+after.mid+after.high
	timbre := 0.0
	if bs > floor && as > floor {
		timbre = .5 * (math.Abs(before.low/bs-after.low/as) + math.Abs(before.mid/bs-after.mid/as) + math.Abs(before.high/bs-after.high/as))
	}
	return math.Max(energy, timbre), timbre
}

func (a *noveltyAccumulator) events() []StructureEvent {
	// Ignore an incomplete final 50 ms frame; it cannot provide sustained context.
	n := len(a.frames)
	result := []StructureEvent{}
	peak := 0.0
	for _, f := range a.frames {
		peak = math.Max(peak, f.power)
	}
	if peak < 1e-8 {
		return result
	}
	floor := math.Max(1e-8, peak*.0004) // -34 dB relative, with an absolute silence floor
	first, last, windowFirst := -1, -1, 0
	activityWindow := max(1, int(math.Round(1.25/a.step)))
	for i, f := range a.frames {
		if f.power < floor {
			continue
		}
		// Require activity separated in time, rather than continuous RMS.
		// Drum-only intros contain silence between kicks; a single short sound
		// should still abstain. The window is bounded and advances only forward.
		for windowFirst < i && (i-windowFirst > activityWindow || a.frames[windowFirst].power < floor) {
			windowFirst++
		}
		if float64(i-windowFirst)*a.step >= .4 {
			if first < 0 {
				first = windowFirst
			}
			last = i
		}
	}
	if first < 0 {
		return result
	}
	result = append(result, StructureEvent{Position: float64(first) * a.step, Kind: "start", Strength: .7, EnergyAfter: math.Sqrt(a.frames[first].power / peak)})
	prefix := make([]noveltyFrame, n+1)
	for i, f := range a.frames {
		prefix[i+1] = prefix[i].add(f)
	}
	mean := func(start, end int) noveltyFrame { return prefix[end].sub(prefix[start]).scale(1 / float64(end-start)) }
	scores := make([]StructureEvent, n)
	// Require contrast in both halves on both sides. Short fills and isolated
	// transients cannot become section cues just because their peak is loud.
	for _, seconds := range []float64{2, 4} {
		w := max(2, int(math.Round(seconds/a.step)))
		h := w / 2
		for i := first + w; i+w <= last+1; i++ {
			before, after := mean(i-w, i), mean(i, i+w)
			strength, timbre := contrast(before, after, floor)
			for _, pair := range [][2]noveltyFrame{{before, mean(i, i+h)}, {before, mean(i+h, i+w)}, {mean(i-w, i-h), after}, {mean(i-h, i), after}} {
				v, _ := contrast(pair[0], pair[1], floor)
				strength = math.Min(strength, v)
			}
			if strength < .30 || strength <= scores[i].Strength {
				continue
			}
			kind := "section"
			if after.power > before.power*1.7 {
				kind = "energy-rise"
			} else if before.power > after.power*1.7 {
				kind = "energy-fall"
			}
			scores[i] = StructureEvent{Position: float64(i) * a.step, Kind: kind, Strength: strength, EnergyBefore: math.Sqrt(before.power / peak), EnergyAfter: math.Sqrt(after.power / peak), TimbreChange: timbre}
		}
	}
	peaks := []StructureEvent{}
	for i := 1; i+1 < n; i++ {
		if scores[i].Strength >= .30 && scores[i].Strength > scores[i-1].Strength && scores[i].Strength >= scores[i+1].Strength {
			peaks = append(peaks, scores[i])
		}
	}
	sort.SliceStable(peaks, func(i, j int) bool { return peaks[i].Strength > peaks[j].Strength })
	for _, e := range peaks {
		if e.Position-result[0].Position < 2 || float64(last)*a.step-e.Position < 2 {
			continue
		}
		close := false
		for _, chosen := range result {
			if math.Abs(e.Position-chosen.Position) < 4 {
				close = true
				break
			}
		}
		if !close {
			e.Strength = energyClamp01(e.Strength)
			result = append(result, e)
		}
		if len(result) == 128 {
			break
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Position < result[j].Position })
	return result
}
