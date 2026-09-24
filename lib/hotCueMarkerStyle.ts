import type { HotCue } from '../slices/djMixerSlice';

/** Canvas marker geometry keeps custom colors while distinguishing generated cues by outline. */
export interface HotCueMarkerStyle {
  color: string;
  fill: boolean;
  dash: number[];
}

export function getHotCueMarkerStyle(cue: Pick<HotCue, 'origin' | 'color'>): HotCueMarkerStyle {
  const generated = cue.origin === 'analysis';
  return {
    color: cue.color || '#22c55e',
    fill: !generated,
    dash: generated ? [2, 2] : [],
  };
}
