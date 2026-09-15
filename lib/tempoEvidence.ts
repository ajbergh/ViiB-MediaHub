/** Rhythm evidence is a diagnostic, not a calibrated probability of correctness. */
export interface TempoEvidence {
  source: 'browser' | 'server';
  bpm?: number;
  score?: number;
  alternateBpm: number | null;
  stability: number | null;
  sections?: number;
  alignment?: GridAlignment | null;
}
export interface GridAlignment {
  matched: number; // fraction of measured onsets within 35ms of a grid beat
  offsetMs: number; // median signed onset-minus-grid residual
  driftMs: number | null; // last minus first section residual, not tempo variability
  sections: number;
}
export interface OnsetSection { start: number; end: number; peaks: number[] }
const median = (values: number[]) => {
  const sorted = [...values].sort((a, b) => a - b);
  return sorted.length ? sorted[Math.floor(sorted.length / 2)] : 0;
};
const wrap = (value: number, period: number) => value - Math.round(value / period) * period;

/** Non-overlapping samples spread over the full track, including intro and outro. */
export function analysisWindows(duration: number): { start: number; duration: number }[] {
  if (!Number.isFinite(duration) || duration <= 0) return [];
  const count = Math.min(5, Math.max(1, Math.floor(duration / 12)));
  const length = Math.min(20, duration / count);
  return Array.from({ length: count }, (_, i) => ({ start: count === 1 ? 0 : (duration - length) * i / (count - 1), duration: length }));
}

/** Positive energy changes reject sustained tones; local maxima preserve transient time. */
export function extractOnsets(samples: Float32Array, rate: number, start: number): number[] {
  const hop = Math.max(1, Math.round(rate * .005));
  const flux: number[] = [];
  let previous = 0;
  for (let i = 0; i + hop <= samples.length; i += hop) {
    let energy = 0;
    for (let j = 0; j < hop; j++) energy += samples[i + j] ** 2;
    const rms = Math.sqrt(energy / hop);
    flux.push(Math.max(0, rms - previous));
    previous = rms;
  }
  const maximum = flux.reduce((a, b) => Math.max(a, b), 0);
  const mean = flux.reduce((a, b) => a + b, 0) / Math.max(1, flux.length);
  if (maximum < 1e-5 || maximum < mean * 4) return [];
  const peaks: number[] = [];
  let last = -Infinity;
  for (let i = 1; i < flux.length - 1; i++) {
    const time = start + i * hop / rate;
    if (flux[i] >= maximum * .12 && flux[i] > mean * 2 && flux[i] > flux[i - 1] && flux[i] >= flux[i + 1] && time - last >= .12) {
      peaks.push(time);
      last = time;
    }
  }
  return peaks;
}

function phaseScore(peaks: number[], bpm: number) {
  const period = 60 / bpm;
  let x = 0, y = 0;
  for (const peak of peaks) {
    const angle = peak / period * 2 * Math.PI;
    x += Math.cos(angle); y += Math.sin(angle);
  }
  const phase = ((Math.atan2(y, x) / (2 * Math.PI) * period) % period + period) % period;
  // Occupancy prevents a double-time grid winning just by adding empty beats.
  const occupied = new Set(peaks.filter(p => Math.abs(wrap(p - phase, period)) <= .035).map(p => Math.round((p - phase) / period))).size;
  const expected = Math.max(1, (peaks[peaks.length - 1] - peaks[0]) / period + 1);
  return { phase, score: Math.hypot(x, y) / peaks.length * Math.min(1, occupied / expected) };
}

export function analyzeOnsetSections(sections: OnsetSection[]) {
  const usable = sections.filter(s => s.peaks.length >= 8 && s.end - s.start >= 4);
  if (!usable.length) return { bpm: 0, confidence: 0, phase: 0, alternateBpm: null, stability: null, sections: 0 };
  const candidates: { bpm: number; score: number }[] = [];
  const winners = usable.map(() => ({ bpm: 0, score: 0 }));
  for (let step = 1200; step <= 4000; step++) {
    const bpm = step / 20;
    let total = 0;
    usable.forEach((section, i) => {
      const { score } = phaseScore(section.peaks, bpm);
      total += score;
      if (score > winners[i].score) winners[i] = { bpm, score };
    });
    candidates.push({ bpm, score: total / usable.length });
  }
  candidates.sort((a, b) => b.score - a.score || a.bpm - b.bpm);
  const best = candidates[0];
  if (best.score < .15) return { bpm: 0, confidence: 0, phase: 0, alternateBpm: null, stability: null, sections: usable.length };
  // Prefer an explicit half/double alternative if supported; otherwise expose a competing tempo.
  const alternate = candidates.find(c => Math.abs(c.bpm - best.bpm) > 2 && (Math.abs(c.bpm * 2 - best.bpm) < .2 || Math.abs(c.bpm / 2 - best.bpm) < .2))
    ?? candidates.find(c => Math.abs(c.bpm - best.bpm) > 2);
  const strongest = usable[winners.reduce((bestIndex, w, i) => w.score > winners[bestIndex].score ? i : bestIndex, 0)];
  return {
    bpm: best.bpm,
    confidence: best.score,
    phase: phaseScore(strongest.peaks, best.bpm).phase,
    alternateBpm: alternate?.bpm ?? null,
    stability: winners.filter(w => Math.abs(w.bpm - best.bpm) <= 1).length / winners.length,
    sections: usable.length,
  };
}

/** Measure against the actual grid, including manual and variable-tempo grids. */
export function measureGridAlignment(sections: OnsetSection[], beats: number[]): GridAlignment | null {
  if (beats.length < 2 || beats.some((b, i) => !Number.isFinite(b) || (i > 0 && b <= beats[i - 1]))) return null;
  const residuals: number[] = [], sectionOffsets: number[] = [];
  for (const section of sections) {
    const local: number[] = [];
    for (const peak of section.peaks) {
      if (peak < beats[0] || peak > beats[beats.length - 1]) continue;
      let low = 0, high = beats.length - 1;
      while (high - low > 1) { const mid = (low + high) >> 1; if (beats[mid] <= peak) low = mid; else high = mid; }
      const residual = peak - (peak - beats[low] <= beats[high] - peak ? beats[low] : beats[high]);
      local.push(residual);
    }
    if (local.length >= 8) { residuals.push(...local); sectionOffsets.push(median(local)); }
  }
  if (!residuals.length) return null;
  return { matched: residuals.filter(r => Math.abs(r) <= .035).length / residuals.length, offsetMs: median(residuals) * 1000,
    driftMs: sectionOffsets.length >= 2 ? (sectionOffsets[sectionOffsets.length - 1] - sectionOffsets[0]) * 1000 : null, sections: sectionOffsets.length };
}
