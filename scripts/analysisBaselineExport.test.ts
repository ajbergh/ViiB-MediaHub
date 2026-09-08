import { describe, expect, it } from 'vitest';
import { browserBaselineResult, indexManifestTracksByFilename } from './analysisBaselineExport';

describe('browser analysis baseline export', () => {
  it('indexes manifests by the selected file name and rejects collisions', () => {
    const indexed = indexManifestTracksByFilename({
      version: 'phase0-v1',
      tracks: [{ id: 'click-120', path: 'C:\\generated\\click-120.wav' }],
    });
    expect(indexed.get('click-120.wav')?.id).toBe('click-120');
    expect(() => indexManifestTracksByFilename({
      version: 'phase0-v1',
      tracks: [
        { id: 'one', path: 'a/same.wav' },
        { id: 'two', path: 'b/same.wav' },
      ],
    })).toThrow('share the filename');
  });

  it('does not convert the browser 120 BPM fallback into a benchmark result', () => {
    expect(browserBaselineResult('silence', { bpm: 120, confidence: 0, peaks: [] }, undefined)).toEqual({ id: 'silence' });
  });

  it('exports valid browser measurements in the analysisbench shape', () => {
    expect(browserBaselineResult(
      'track',
      { bpm: 128.5, confidence: 0.8, peaks: [] },
      { key: 'Am', keyFull: 'A minor', isMinor: true, tonic: 9, confidence: 0.7, camelot: '8A', openKey: '6m' },
    )).toEqual({ id: 'track', bpm: 128.5, confidence: 0.8, tempoConfidence: 0.8, key: 'A minor', keyConfidence: 0.7 });
  });
});
