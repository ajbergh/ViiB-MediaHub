import { validBeatGrid, type BeatGridSource } from './beatGridConfidence';

export interface TestMixDownbeatEvidence {
  beats: number[] | null | undefined;
  downbeatIndices: number[] | null | undefined;
  locked: boolean;
  source: BeatGridSource | string;
}

export interface TestMixPhaseEvidenceResult {
  ready: boolean;
  reason: string;
}

/**
 * A readiness check for human-reviewed bar anchors, not a claim that the
 * HTML-media Test Mix transport can start or remain sample-accurate.
 */
export function inspectTestMixDownbeatEvidence(evidence: TestMixDownbeatEvidence): TestMixPhaseEvidenceResult {
  if (evidence.source !== 'manual') return { ready: false, reason: 'grid is not manually reviewed' };
  if (!evidence.locked) return { ready: false, reason: 'grid is not locked' };
  if (!validBeatGrid(evidence.beats)) return { ready: false, reason: 'beat grid is missing or invalid' };
  const indices = evidence.downbeatIndices;
  if (!indices?.length) return { ready: false, reason: 'explicit downbeat indices are missing' };
  if (!indices.every((index, position) => Number.isInteger(index) && index >= 0 && index < evidence.beats!.length
    && (position === 0 || index > indices[position - 1]))) {
    return { ready: false, reason: 'downbeat indices are invalid' };
  }
  return { ready: true, reason: 'manual locked grid with explicit downbeats' };
}

export function describeTestMixPhaseEvidence(
  reference: TestMixDownbeatEvidence,
  candidate: TestMixDownbeatEvidence | null,
): string {
  const referenceResult = inspectTestMixDownbeatEvidence(reference);
  const candidateResult = candidate
    ? inspectTestMixDownbeatEvidence(candidate)
    : { ready: false, reason: 'grid is unavailable' };
  if (!referenceResult.ready) return `Downbeat evidence unavailable: reference ${referenceResult.reason}.`;
  if (!candidateResult.ready) return `Downbeat evidence unavailable: candidate ${candidateResult.reason}.`;
  return 'Manual downbeat evidence is available for both tracks; phase lock is unavailable because these HTML-audio sources have no shared scheduled start (and phrase length is not recorded).';
}
