import { describe, expect, it } from 'vitest';
import { formatDJStemStatus, type DJStemStatus } from './DJStemControls';

const availableStatus: DJStemStatus = {
  mode: 'full',
  available: true,
  bufferedSeconds: 0.8,
  underruns: 0,
  supportsKeyLock: false,
  supportsScratch: false,
  supportsSampleAccurateLoop: false,
};

describe('DJ stem status copy', () => {
  it('reports unavailable packages and explicit full-track fallback', () => {
    expect(formatDJStemStatus({ ...availableStatus, available: false })).toBe('Full track · no stem package');
    expect(formatDJStemStatus({ ...availableStatus, mode: 'fallback', error: 'decoder failed' })).toBe('Full track · stems unavailable (decoder failed)');
  });

  it('makes buffering, ready, and underrun states visible', () => {
    expect(formatDJStemStatus({ ...availableStatus, mode: 'stems', bufferedSeconds: 0.05 })).toBe('Stems · buffering');
    expect(formatDJStemStatus({ ...availableStatus, mode: 'stems', bufferedSeconds: 1.25 })).toBe('Stems ready · 1.3 s buffered');
    expect(formatDJStemStatus({ ...availableStatus, mode: 'stems', underruns: 2 })).toBe('Stems · 2 audio underruns');
  });
});
