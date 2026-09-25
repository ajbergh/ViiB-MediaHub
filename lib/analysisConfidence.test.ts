import { describe, expect, it } from 'vitest';
import type { TrackAnalysisFeature } from '../services/api';
import {
  CURRENT_ENERGY_ALGORITHM_VERSION,
  formatConfidenceEvidence,
  getAnalysisConfidenceEvidence,
  normalizeConfidence,
} from './analysisConfidence';

const analysis = (overrides: Partial<TrackAnalysisFeature> = {}): TrackAnalysisFeature => ({
  songId: 'song',
  status: 'complete',
  bpmSource: 'measured',
  keySource: 'measured',
  syncAllowed: true,
  ...overrides,
});

describe('analysis confidence evidence', () => {
  it('normalizes only finite scores in the inclusive 0–1 range', () => {
    expect(normalizeConfidence(0)).toBe(0);
    expect(normalizeConfidence(1)).toBe(1);
    expect(normalizeConfidence(0.625)).toBe(0.625);
    expect(normalizeConfidence(undefined)).toBeNull();
    expect(normalizeConfidence(Number.NaN)).toBeNull();
    expect(normalizeConfidence(Number.POSITIVE_INFINITY)).toBeNull();
    expect(normalizeConfidence(-0.01)).toBeNull();
    expect(normalizeConfidence(1.01)).toBeNull();
  });

  it('labels raw measured BPM and Key evidence and formats it without percent or probability claims', () => {
    const evidence = getAnalysisConfidenceEvidence(analysis({ bpmConfidence: 0.731, keyConfidence: 0.4 }));
    expect(evidence.bpm.sourceLabel).toBe('Measured');
    expect(evidence.bpm.value).toBe(0.731);
    expect(formatConfidenceEvidence(evidence.bpm)).toBe('Measured 0.73');
    expect(formatConfidenceEvidence(evidence.key)).toBe('Measured 0.40');
    expect(evidence.bpm.description).toContain('not a probability');
  });

  it('does not inherit measured confidence when BPM or Key is manually overridden', () => {
    const evidence = getAnalysisConfidenceEvidence(analysis({
      bpmSource: 'manual', bpmConfidence: 0.9,
      keySource: 'manual', keyConfidence: 0.8,
    }));
    expect(evidence.bpm.sourceLabel).toBe('Manual');
    expect(evidence.bpm.value).toBeNull();
    expect(formatConfidenceEvidence(evidence.bpm)).toBe('Manual');
    expect(evidence.key.sourceLabel).toBe('Manual');
    expect(evidence.key.value).toBeNull();
  });

  it('shows unknown or missing BPM and Key source confidence as an em dash', () => {
    const evidence = getAnalysisConfidenceEvidence(analysis({
      bpmSource: 'unknown', bpmConfidence: 0.9,
      keySource: 'unknown', keyConfidence: 0.9,
    }));
    expect(formatConfidenceEvidence(evidence.bpm)).toBe('—');
    expect(formatConfidenceEvidence(evidence.key)).toBe('—');
  });

  it('keeps a measured dimension visible when another dimension is missing on a partial row', () => {
    const evidence = getAnalysisConfidenceEvidence(analysis({
      status: 'partial',
      bpmConfidence: 0.72,
      keySource: 'unknown',
    }));
    expect(formatConfidenceEvidence(evidence.bpm)).toBe('Measured 0.72');
    expect(formatConfidenceEvidence(evidence.key)).toBe('—');
    expect(formatConfidenceEvidence(evidence.energy)).toBe('—');
  });

  it('labels current settled Energy evidence as heuristic, and hides missing, stale, or unsettled values', () => {
    const current = analysis({
      energyLevel: 7,
      energyLevelConfidence: 0.65,
      energyAlgorithmVersion: CURRENT_ENERGY_ALGORITHM_VERSION,
    });
    const evidence = getAnalysisConfidenceEvidence(current).energy;
    expect(evidence.sourceLabel).toBe('Heuristic');
    expect(formatConfidenceEvidence(evidence)).toBe('Heuristic 0.65');
    expect(evidence.description).toContain('not a probability');

    expect(formatConfidenceEvidence(getAnalysisConfidenceEvidence(analysis({
      ...current, energyAlgorithmVersion: 'energy-level-old',
    })).energy)).toBe('—');
    expect(formatConfidenceEvidence(getAnalysisConfidenceEvidence(analysis({
      ...current, status: 'running',
    })).energy)).toBe('—');
    expect(formatConfidenceEvidence(getAnalysisConfidenceEvidence(analysis({
      ...current, energyLevel: undefined,
    })).energy)).toBe('—');
    expect(formatConfidenceEvidence(getAnalysisConfidenceEvidence(analysis({
      ...current, energyLevelConfidence: 1.2,
    })).energy)).toBe('—');
  });

  it('keeps valid failed-row Energy heuristic evidence while unavailable BPM and Key stay missing', () => {
    const evidence = getAnalysisConfidenceEvidence(analysis({
      status: 'failed',
      bpmSource: 'unknown',
      keySource: 'unknown',
      energyLevel: 4,
      energyLevelConfidence: 0.55,
      energyAlgorithmVersion: CURRENT_ENERGY_ALGORITHM_VERSION,
    }));
    expect(formatConfidenceEvidence(evidence.bpm)).toBe('—');
    expect(formatConfidenceEvidence(evidence.key)).toBe('—');
    expect(formatConfidenceEvidence(evidence.energy)).toBe('Heuristic 0.55');
  });

});
