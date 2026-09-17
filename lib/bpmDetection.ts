import { analysisWindows, extractOnsets, analyzeOnsetSections, measureGridAlignment, type OnsetSection, type TempoEvidence } from './tempoEvidence';

const MIN_BPM = 60;
const MAX_BPM = 200;
export interface BPMResult {
  bpm: number;
  confidence: number;
  peaks: number[];
  beatGrid: number[];
  evidence: TempoEvidence;
  onsetSections: OnsetSection[];
}

/** Bounded offline windows cover the track, rather than trusting a quiet intro. */
export async function detectBPM(audioUrl: string, onProgress?: (progress: number) => void): Promise<BPMResult> {
  onProgress?.(.05);
  const response = await fetch(audioUrl);
  if (!response.ok) throw new Error(`Failed to fetch audio: ${response.status}`);
  const bytes = await response.arrayBuffer();
  const decoder = new AudioContext();
  let buffer: AudioBuffer;
  try { buffer = await decoder.decodeAudioData(bytes); } finally { await decoder.close(); }
  const windows = analysisWindows(buffer.duration);
  const sections: OnsetSection[] = [];
  for (const [index, window] of windows.entries()) {
    const offline = new OfflineAudioContext(1, Math.max(1, Math.ceil(window.duration * 11025)), 11025);
    const source = offline.createBufferSource();
    source.buffer = buffer;
    // Preserve bass and midrange percussion instead of relying only on sub-bass kicks.
    const filter = offline.createBiquadFilter();
    filter.type = 'lowpass'; filter.frequency.value = 1800;
    source.connect(filter); filter.connect(offline.destination);
    source.start(0, window.start, window.duration);
    const rendered = await offline.startRendering();
    sections.push({ start: window.start, end: window.start + window.duration, peaks: extractOnsets(rendered.getChannelData(0), rendered.sampleRate, window.start) });
    onProgress?.(.15 + .65 * (index + 1) / windows.length);
    // Give transport/UI work a chance between bounded analysis windows.
    await new Promise<void>(resolve => setTimeout(resolve, 0));
  }
  const result = analyzeOnsetSections(sections);
  const beatGrid = result.bpm > 0 ? generateBeatGrid(result.bpm, buffer.duration, result.phase) : [];
  onProgress?.(1);
  return { bpm: result.bpm, confidence: result.confidence, peaks: sections.flatMap(s => s.peaks), beatGrid, onsetSections: sections,
    evidence: { source: 'browser', bpm: result.bpm, score: result.confidence, alternateBpm: result.alternateBpm, stability: result.stability, sections: result.sections, alignment: measureGridAlignment(sections, beatGrid) } };
}

/**
 * Generates a beat grid based on BPM and track duration.
 * 
 * @param bpm - Detected BPM
 * @param duration - Track duration in seconds
 * @param firstBeatOffset - Time of first beat in seconds (default 0)
 * @returns Array of beat positions in seconds
 */
export function generateBeatGrid(
  bpm: number,
  duration: number,
  firstBeatOffset: number = 0
): number[] {
  if (!Number.isFinite(bpm) || bpm <= 0 || !Number.isFinite(duration) || duration <= 0 || !Number.isFinite(firstBeatOffset) || firstBeatOffset < 0) return [];
  const beatInterval = 60 / bpm; // seconds per beat
  const beats: number[] = [];
  
  for (let t = firstBeatOffset; t < duration; t += beatInterval) {
    beats.push(t);
  }
  
  return beats;
}

/**
 * Adjusts BPM to avoid half/double time errors.
 * If the detected BPM seems like half or double time, correct it.
 * 
 * @param bpm - Detected BPM
 * @param targetRange - Preferred BPM range [min, max]
 * @returns Adjusted BPM
 */
export function normalizeBPM(
  bpm: number,
  targetRange: [number, number] = [80, 160]
): number {
  const [min, max] = targetRange;
  
  // Double if too slow
  while (bpm < min && bpm * 2 <= MAX_BPM) {
    bpm *= 2;
  }
  
  // Halve if too fast
  while (bpm > max && bpm / 2 >= MIN_BPM) {
    bpm /= 2;
  }
  
  return bpm;
}
