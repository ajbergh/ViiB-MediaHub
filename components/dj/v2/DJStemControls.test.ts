import { describe, expect, it } from 'vitest';
import {
  formatDJStemStatus,
  stemPresetMuteState,
  transitionDJStemPreset,
  type DJStemMuteState,
  type DJStemStatus,
} from './DJStemControls';

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
    expect(formatDJStemStatus({ ...availableStatus, mode: 'fallback', available: false })).toBe('Full track · no stem package');
  });

  it('makes buffering, ready, and underrun states visible', () => {
    expect(formatDJStemStatus({ ...availableStatus, mode: 'stems', bufferedSeconds: 0.05 })).toBe('Stems · buffering');
    expect(formatDJStemStatus({ ...availableStatus, mode: 'stems', bufferedSeconds: 1.25 })).toBe('Stems ready · 1.3 s buffered');
    expect(formatDJStemStatus({ ...availableStatus, mode: 'stems', underruns: 2 })).toBe('Stems · 2 audio underruns');
  });
});

describe('DJ stem mix presets', () => {
  it('sets full, acapella and instrumental mutes without modeling or changing gains', () => {
    expect(stemPresetMuteState('full')).toEqual({ vocals: false, drums: false, bass: false, music: false });
    expect(stemPresetMuteState('acapella')).toEqual({ vocals: false, drums: true, bass: true, music: true });
    expect(stemPresetMuteState('instrumental')).toEqual({ vocals: true, drums: false, bass: false, music: false });
  });

  it('restores the original mute state when toggled off and keeps the first snapshot when switching presets', () => {
    const original: DJStemMuteState = { vocals: true, drums: false, bass: true, music: false };
    const acapella = transitionDJStemPreset(null, 'acapella', original, null);
    expect(acapella.muteState).toEqual(stemPresetMuteState('acapella'));
    expect(acapella.restoreState).toEqual(original);

    const instrumental = transitionDJStemPreset(acapella.activePreset, 'instrumental', acapella.muteState, acapella.restoreState);
    expect(instrumental.muteState).toEqual(stemPresetMuteState('instrumental'));
    expect(instrumental.restoreState).toEqual(original);

    const restored = transitionDJStemPreset(instrumental.activePreset, 'instrumental', instrumental.muteState, instrumental.restoreState);
    expect(restored.activePreset).toBeNull();
    expect(restored.muteState).toEqual(original);
    expect(restored.restoreState).toBeNull();
  });

  it('captures the current mutes as the restore state for a fresh preset', () => {
    const first = transitionDJStemPreset(null, 'full', { vocals: true, drums: false, bass: false, music: false }, null);
    const manuallyEdited = { ...first.muteState, vocals: true };
    const next = transitionDJStemPreset(null, 'acapella', manuallyEdited, null);
    expect(next.restoreState).toEqual(manuallyEdited);
  });
});
