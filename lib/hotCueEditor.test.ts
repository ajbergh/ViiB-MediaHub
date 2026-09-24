import { describe, expect, it } from 'vitest';
import { convertHotCueToManual, moveHotCueToPosition, recolorHotCue, renameHotCue } from './hotCueEditor';
import type { HotCue } from '../slices/djMixerSlice';

const generatedCue: HotCue = {
  slot: 3,
  position: 12.5,
  label: 'Drop',
  color: '#123456',
  origin: 'analysis',
  generatorVersion: 'cue-v2',
  confidence: 0.91,
  kind: 'drop',
  locked: true,
  rationale: 'measured-energy-transition',
  sourceFingerprint: 'audio-sha256',
  downbeatAligned: false,
  updatedAt: 1725000000123,
};

describe('cue editor actions', () => {
  it('renames and recolors without changing generated provenance or metadata', () => {
    const renamed = renameHotCue(generatedCue, 'Breakdown');
    const recolored = recolorHotCue(renamed, '#abcdef');
    expect(recolored).toEqual({ ...generatedCue, label: 'Breakdown', color: '#abcdef' });
  });

  it('converts only on request and removes generated-only metadata', () => {
    expect(convertHotCueToManual(generatedCue)).toEqual({
      slot: 3,
      position: 12.5,
      label: 'Drop',
      color: '#123456',
      origin: 'user',
      locked: true,
      updatedAt: 1725000000123,
    });
  });

  it('keeps Move manualizing a generated cue and clears generated metadata', () => {
    expect(moveHotCueToPosition(generatedCue, 18)).toEqual({
      slot: 3,
      position: 18,
      label: 'Drop',
      color: '#123456',
      origin: 'user',
    });
  });
});
