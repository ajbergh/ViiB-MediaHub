import { describe, expect, it } from 'vitest';
import { getHotCueMarkerStyle } from './hotCueMarkerStyle';

describe('waveform hot-cue provenance markers', () => {
  it('keeps user cues solid and preserves their custom color', () => {
    expect(getHotCueMarkerStyle({ origin: 'user', color: '#ff00aa' })).toEqual({
      color: '#ff00aa', fill: true, dash: [],
    });
  });

  it('renders generated analysis cues as dashed outlines with the same position color contract', () => {
    expect(getHotCueMarkerStyle({ origin: 'analysis', color: '#33aaff' })).toEqual({
      color: '#33aaff', fill: false, dash: [2, 2],
    });
    expect(getHotCueMarkerStyle({ origin: 'analysis', color: '' })).toEqual({
      color: '#22c55e', fill: false, dash: [2, 2],
    });
  });
});
