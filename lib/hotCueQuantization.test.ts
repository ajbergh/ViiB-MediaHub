import { describe, expect, it } from 'vitest';
import { quantizeHotCuePosition, type HotCueQuantizeMode } from './hotCueQuantization';

describe('hot cue quantization', () => {
  it.each([
    ['beat', 1.7, 2],
    ['half', 1.7, 1.5],
    ['quarter', 1.7, 1.75],
  ] as [Exclude<HotCueQuantizeMode, 'off'>, number, number][])('snaps %s positions to the nearest grid subdivision', (mode, position, expected) => {
    expect(quantizeHotCuePosition({ position }, [0, 1, 2], mode)).toBe(expected);
  });

  it('does nothing when quantization is off or the cue is already on-grid', () => {
    expect(quantizeHotCuePosition({ position: 0.6 }, [0, 1], 'off')).toBeNull();
    expect(quantizeHotCuePosition({ position: 1 }, [0, 1], 'beat')).toBeNull();
  });

  it('interpolates subdivisions from variable beat intervals', () => {
    expect(quantizeHotCuePosition({ position: 1.8 }, [0, 1, 3], 'half')).toBe(2);
    expect(quantizeHotCuePosition({ position: 2.3 }, [0, 1, 3], 'quarter')).toBe(2.5);
  });

  it('includes grid endpoints and leaves positions outside the stored grid unchanged', () => {
    expect(quantizeHotCuePosition({ position: 0 }, [0, 0.8, 2], 'quarter')).toBeNull();
    expect(quantizeHotCuePosition({ position: 2 }, [0, 0.8, 2], 'beat')).toBeNull();
    expect(quantizeHotCuePosition({ position: -0.01 }, [0, 1], 'quarter')).toBeNull();
    expect(quantizeHotCuePosition({ position: 1.01 }, [0, 1], 'quarter')).toBeNull();
  });

  it('returns no move for missing, malformed, or locked grid/cue data', () => {
    for (const grid of [null, [], [0], [0, Number.NaN], [0, 0], [1, 0]]) {
      expect(quantizeHotCuePosition({ position: 0.4 }, grid, 'quarter')).toBeNull();
    }
    expect(quantizeHotCuePosition({ position: 0.4, locked: true }, [0, 1], 'quarter')).toBeNull();
    expect(quantizeHotCuePosition({ position: Number.NaN }, [0, 1], 'quarter')).toBeNull();
  });
});
