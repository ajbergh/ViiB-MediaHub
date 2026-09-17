import { describe, expect, it } from 'vitest';
import { analysisWindows, analyzeOnsetSections, extractOnsets, measureGridAlignment } from './tempoEvidence';
import { generateBeatGrid } from './bpmDetection';

function sections(bpm: number, phase = .13) {
  const peaks = generateBeatGrid(bpm, 180, phase);
  return analysisWindows(180).map(w => ({ start: w.start, end: w.start + w.duration, peaks: peaks.filter(p => p >= w.start && p < w.start + w.duration) }));
}
describe('multi-section tempo evidence', () => {
  it('covers the end of long tracks without overlapping short tracks', () => {
    expect(analysisWindows(180).at(-1)).toEqual({ start: 160, duration: 20 });
    expect(analysisWindows(30)).toEqual([{ start: 0, duration: 15 }, { start: 15, duration: 15 }]);
    expect(analysisWindows(NaN)).toEqual([]);
  });
  it('finds fractional tempo after a silent intro and retains a half-time alternative', () => {
    const input = sections(128.5);
    input[0].peaks = [];
    const result = analyzeOnsetSections(input);
    expect(result.bpm).toBeCloseTo(128.5, 1);
    expect(result.phase).toBeCloseTo(.13, 2);
    expect(result.stability).toBe(1);
    expect(result.sections).toBe(4);
    expect(result.alternateBpm).toBeCloseTo(64.25, 0);
  });
  it('tolerates missing kicks without doubling or halving the primary', () => {
    const input = sections(126);
    input.forEach(s => { s.peaks = s.peaks.filter((_, i) => i % 7 !== 0); });
    expect(analyzeOnsetSections(input).bpm).toBeCloseTo(126, 1);
  });
  it('reports disagreement when tempo changes across the track', () => {
    const a = sections(120), b = sections(132);
    const result = analyzeOnsetSections([a[0], a[1], a[2], b[3], b[4]]);
    expect(result.stability).toBeLessThan(1);
  });
  it('refuses silence and insufficient evidence instead of inventing 120 BPM', () => {
    expect(extractOnsets(new Float32Array(11025 * 20), 11025, 0)).toEqual([]);
    expect(analyzeOnsetSections([{ start: 0, end: 20, peaks: [1, 2] }]).bpm).toBe(0);
    expect(generateBeatGrid(0, 20)).toEqual([]);
  });
  it('extracts pulse timing from PCM and rejects a sustained tone', () => {
    const rate = 11025, pcm = new Float32Array(rate * 20);
    for (const time of generateBeatGrid(120, 20, .2)) {
      const start = Math.round(time * rate);
      for (let i = 0; i < 200 && start + i < pcm.length; i++) pcm[start + i] = Math.sin(i * .2) * Math.exp(-i / 30);
    }
    const peaks = extractOnsets(pcm, rate, 0);
    expect(peaks.length).toBeGreaterThan(30);
    expect(analyzeOnsetSections([{ start: 0, end: 20, peaks }]).bpm).toBeCloseTo(120, 0);
    const tone = Float32Array.from({ length: rate * 20 }, (_, i) => .5 * Math.sin(2 * Math.PI * 100 * i / rate));
    expect(extractOnsets(tone, rate, 0).length).toBeLessThan(8);
  });
});
describe('actual grid alignment', () => {
  it('distinguishes a constant offset from accumulated drift', () => {
    const input = sections(120);
    const shifted = measureGridAlignment(input, generateBeatGrid(120, 180, .10))!;
    expect(shifted.offsetMs).toBeCloseTo(30, 2);
    expect(shifted.driftMs).toBeCloseTo(0, 2);
    const drifting = measureGridAlignment(input, generateBeatGrid(120.1, 180, .13))!;
    expect(Math.abs(drifting.driftMs!)).toBeGreaterThan(100);
    expect(drifting.matched).toBeLessThan(shifted.matched);
  });
  it('does not infer drift from one section or missing grids', () => {
    expect(measureGridAlignment(sections(120).slice(0, 1), generateBeatGrid(120, 180, .13))?.driftMs).toBeNull();
    expect(measureGridAlignment(sections(120), [])).toBeNull();
  });
});
