import type { HotCue } from '../slices/djMixerSlice';

export function renameHotCue(cue: HotCue, label: string): HotCue {
  return { ...cue, label };
}

export function recolorHotCue(cue: HotCue, color: string): HotCue {
  return { ...cue, color };
}

export function convertHotCueToManual(cue: HotCue): HotCue {
  const {
    generatorVersion: _generatorVersion,
    confidence: _confidence,
    kind: _kind,
    rationale: _rationale,
    sourceFingerprint: _sourceFingerprint,
    downbeatAligned: _downbeatAligned,
    ...manualCue
  } = cue;
  return { ...manualCue, origin: 'user' };
}

export function moveHotCueToPosition(cue: HotCue, position: number): HotCue {
  return {
    slot: cue.slot,
    position,
    label: cue.label || cue.kind || `Cue ${cue.slot}`,
    color: cue.color,
    origin: 'user',
  };
}
