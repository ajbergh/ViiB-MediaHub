import { describe, expect, it } from 'vitest';
import { calculateBeatJumpTimestamp, getBeatJumpControlAmounts } from './beatJump';

describe('beat jump timestamp', () => {
  it('uses stored timestamps for straight and variable beat grids', () => {
    expect(calculateBeatJumpTimestamp({ position: 0.2, beats: 2, duration: 20, beatGrid: [0, 0.5, 1, 1.5], bpm: 120 })).toBe(1);
    expect(calculateBeatJumpTimestamp({ position: 1.1, beats: 1, duration: 20, beatGrid: [0, 0.5, 1.2, 2], bpm: 120 })).toBe(2);
  });

  it('uses the nearest grid beat from arbitrary playhead positions and breaks ties earlier', () => {
    expect(calculateBeatJumpTimestamp({ position: 0.61, beats: 1, duration: 10, beatGrid: [0, 1, 2], bpm: 120 })).toBe(2);
    expect(calculateBeatJumpTimestamp({ position: 0.5, beats: 1, duration: 10, beatGrid: [0, 1, 2], bpm: 120 })).toBe(1);
  });

  it('jumps backward and forward by the requested beat count', () => {
    const options = { duration: 10, beatGrid: [0, 1, 2, 3, 4], bpm: 120 };
    expect(calculateBeatJumpTimestamp({ ...options, position: 3.1, beats: -2 })).toBe(1);
    expect(calculateBeatJumpTimestamp({ ...options, position: 0.1, beats: 3 })).toBe(3);
  });

  it('applies deck-local beat-grid offsets to beat matching and targets', () => {
    expect(calculateBeatJumpTimestamp({ position: 2.1, beats: 1, duration: 10, beatGrid: [0, 1, 2, 3], beatGridOffset: 0.25, bpm: 120 })).toBe(3.25);
    expect(calculateBeatJumpTimestamp({ position: 1.1, beats: 1, duration: 10, beatGrid: [0, 1, 2, 3], beatGridOffset: -0.5, bpm: 120 })).toBe(2.5);
    expect(calculateBeatJumpTimestamp({ position: 1.6, beats: 1, duration: 10, beatGrid: [0, 1, 2, 3], beatGridOffset: 0.5, bpm: 120 })).toBe(2.5);
  });

  it('clamps grid and BPM targets to the track duration', () => {
    expect(calculateBeatJumpTimestamp({ position: 2, beats: 1, duration: 2.5, beatGrid: [0, 1, 2], beatGridOffset: 1, bpm: 120 })).toBe(2.5);
    expect(calculateBeatJumpTimestamp({ position: 0.1, beats: -4, duration: 3, bpm: 120 })).toBe(0);
    expect(calculateBeatJumpTimestamp({ position: 2.9, beats: 4, duration: 3, bpm: 120 })).toBe(3);
  });

  it('falls back to the legacy BPM calculation when grid data is unavailable', () => {
    for (const beatGrid of [null, [], [0], [0, Number.NaN], [0, 0], [1, 0], [0, -1]]) {
      expect(calculateBeatJumpTimestamp({ position: 1, beats: 4, duration: 10, beatGrid, bpm: 120 })).toBe(3);
    }
    expect(calculateBeatJumpTimestamp({ position: 1, beats: 1, duration: 10, beatGrid: [0, 1, 2], bpm: 120, beatGridOffset: Number.NaN })).toBe(1.5);
    expect(calculateBeatJumpTimestamp({ position: 1, beats: 4, duration: 10, beatGrid: [0, 1, 2], bpm: 120 })).toBe(3);
  });

  it('uses BPM outside the stored grid and returns null when neither timing source is usable', () => {
    expect(calculateBeatJumpTimestamp({ position: 4, beats: 1, duration: 10, beatGrid: [0, 1, 2], bpm: 120 })).toBe(4.5);
    expect(calculateBeatJumpTimestamp({ position: -2, beats: 1, duration: 10, beatGrid: [0, 1, 2], bpm: 120 })).toBe(0);
    expect(calculateBeatJumpTimestamp({ position: 1, beats: 1, duration: 10, beatGrid: [0, 1, 2] })).toBe(2);
    expect(calculateBeatJumpTimestamp({ position: 1, beats: 1, duration: 10, beatGrid: [0, 1, 2], bpm: 0 })).toBe(2);
    expect(calculateBeatJumpTimestamp({ position: 1, beats: 1, duration: 10, beatGrid: [0, 0], bpm: 0 })).toBeNull();
    expect(calculateBeatJumpTimestamp({ position: Number.NaN, beats: 1, duration: 10, bpm: 120 })).toBeNull();
  });
});

describe('DJ beat jump controls', () => {
  it('exposes symmetric compact and full backward/forward controls through 32 beats', () => {
    expect(getBeatJumpControlAmounts(true, 'back')).toEqual([-32, -4, -1]);
    expect(getBeatJumpControlAmounts(true, 'forward')).toEqual([1, 4, 32]);
    expect(getBeatJumpControlAmounts(false, 'back')).toEqual([-32, -16, -8, -4, -1]);
    expect(getBeatJumpControlAmounts(false, 'forward')).toEqual([1, 4, 8, 16, 32]);
  });
});
