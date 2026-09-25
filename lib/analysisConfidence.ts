import type { TrackAnalysisFeature } from '../services/api';

export const CURRENT_ENERGY_ALGORITHM_VERSION = 'energy-level-v1-fixed-reference';

export interface ConfidenceEvidence {
  sourceLabel: 'Measured' | 'Manual' | 'Heuristic' | null;
  value: number | null;
  description: string;
}

export interface AnalysisConfidenceEvidence {
  bpm: ConfidenceEvidence;
  key: ConfidenceEvidence;
  energy: ConfidenceEvidence;
}

const MISSING_EVIDENCE: ConfidenceEvidence = {
  sourceLabel: null,
  value: null,
  description: 'No valid confidence evidence is available.',
};

/** Accept only normalized finite scores. Scores are raw evidence, not probabilities. */
export function normalizeConfidence(value?: number): number | null {
  return typeof value === 'number' && Number.isFinite(value) && value >= 0 && value <= 1
    ? value
    : null;
}

function measuredEvidence(value: number | undefined, source: string, dimension: 'BPM' | 'Key'): ConfidenceEvidence {
  if (source === 'manual') {
    return {
      sourceLabel: 'Manual',
      value: null,
      description: `The effective ${dimension} is manual; no measured detector score is attributed to it.`,
    };
  }
  if (source !== 'measured') return MISSING_EVIDENCE;

  const normalized = normalizeConfidence(value);
  return normalized === null
    ? MISSING_EVIDENCE
    : {
        sourceLabel: 'Measured',
        value: normalized,
        description: `Raw measured ${dimension} detector score on a 0–1 scale; not a probability.`,
      };
}

export function getAnalysisConfidenceEvidence(
  analysis?: TrackAnalysisFeature,
): AnalysisConfidenceEvidence {
  const bpm = measuredEvidence(analysis?.bpmConfidence, analysis?.bpmSource ?? 'unknown', 'BPM');
  const key = measuredEvidence(analysis?.keyConfidence, analysis?.keySource ?? 'unknown', 'Key');
  const settled = analysis?.status === 'complete' || analysis?.status === 'partial' || analysis?.status === 'failed';
  const energyConfidence = normalizeConfidence(analysis?.energyLevelConfidence);
  const hasCurrentEnergy = settled
    && analysis?.energyLevel !== undefined
    && Number.isFinite(analysis.energyLevel)
    && analysis.energyLevel >= 1
    && analysis.energyLevel <= 10
    && analysis.energyAlgorithmVersion === CURRENT_ENERGY_ALGORITHM_VERSION;
  const energy: ConfidenceEvidence = hasCurrentEnergy && energyConfidence !== null
    ? {
        sourceLabel: 'Heuristic',
        value: energyConfidence,
        description: 'Energy confidence is an evidence-availability heuristic on a 0–1 scale, not a probability.',
      }
    : MISSING_EVIDENCE;

  return { bpm, key, energy };
}

export function formatConfidenceEvidence(evidence: ConfidenceEvidence): string {
  if (evidence.sourceLabel === 'Manual') return 'Manual';
  if (evidence.sourceLabel === null || evidence.value === null) return '—';
  return `${evidence.sourceLabel} ${evidence.value.toFixed(2)}`;
}

