import { describe, expect, it } from 'vitest';
import {
  normalizeAnalysisCueList,
  normalizeTrackEnergyFeatures,
  normalizeTrackTransitionRecommendations,
} from './api';

describe('DJ analysis response normalization', () => {
  it('turns missing, null, and malformed energy arrays into safe display values', () => {
    const normalized = normalizeTrackEnergyFeatures({
      songId: 'song', integratedLufs: null, truePeakDbfs: 'peak', energy: null,
      sections: [{ start: 0, end: 'bad', energy: 0.5 }], cueSuggestions: null,
    });
    expect(normalized.energy).toEqual([]);
    expect(normalized.sections).toEqual([]);
    expect(normalized.cueSuggestions).toEqual([]);
    expect(Number.isNaN(normalized.integratedLufs)).toBe(true);
    expect(Number.isNaN(normalized.truePeakDbfs)).toBe(true);
  });

  it('normalizes partial recommendation objects and drops invalid candidates', () => {
    const normalized = normalizeTrackTransitionRecommendations({
      recommendations: [
        { songId: 'candidate', title: 'Candidate', artist: 'Artist', score: 0.8, vector: {}, components: null },
        { title: 'Broken candidate' },
      ],
    });
    expect(normalized.recommendations).toHaveLength(1);
    expect(normalized.recommendations[0].components).toEqual([]);
    expect(normalized.recommendations[0].filterEvidence).toEqual({});
  });

  it('normalizes partial cue responses before cue-editor rendering', () => {
    const normalized = normalizeAnalysisCueList({ songId: 'song', hotCues: null, generatedCandidates: null, suppressions: null });
    expect(normalized.sourceFingerprint).toBe('');
    expect(normalized.hotCues).toEqual([]);
    expect(normalized.generatedCandidates).toEqual([]);
    expect(normalized.suppressions).toEqual([]);
  });
});
