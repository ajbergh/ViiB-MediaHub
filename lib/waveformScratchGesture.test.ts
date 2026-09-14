import { describe, expect, it } from 'vitest';
import { WaveformScratchGesture } from './waveformScratchGesture';

describe('waveform scratch gesture', () => {
  it('moves the waveform under the hand with zoom-scaled time', () => {
    const gesture = new WaveformScratchGesture(500, 0, 10 / 1000);
    expect(gesture.move(550, 50).delta).toBe(-0.5);
    expect(gesture.move(450, 100).delta).toBe(1);
    const zoomed = new WaveformScratchGesture(500, 0, 2 / 1000);
    expect(zoomed.move(550, 50).delta).toBe(-0.1);
  });

  it('reverses release momentum immediately and bounds fast flicks', () => {
    const gesture = new WaveformScratchGesture(500, 0, 0.01);
    gesture.move(600, 16);
    expect(gesture.release(20, false)).toBeLessThan(0);
    gesture.move(400, 32);
    expect(gesture.release(36, false)).toBe(8);
  });

  it('does not coast on a stationary grab, a held release, or cancellation', () => {
    const gesture = new WaveformScratchGesture(500, 0, 0.01);
    expect(gesture.release(10, false)).toBe(0);
    gesture.move(550, 30);
    expect(gesture.release(40, true)).toBe(0);
    expect(gesture.release(110, false)).toBe(0);
  });

  it('tracks independent deck gestures without sharing velocity or position', () => {
    const a = new WaveformScratchGesture(500, 0, 0.01);
    const b = new WaveformScratchGesture(500, 0, 0.01);
    a.move(600, 20);
    b.move(400, 20);
    expect(a.release(30, false)).toBeLessThan(0);
    expect(b.release(30, false)).toBeGreaterThan(0);
  });
});
