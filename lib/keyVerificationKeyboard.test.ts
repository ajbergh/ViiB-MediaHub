import { describe, expect, it, vi } from 'vitest';
import { keyReferenceFrequency, keyScalePitchClasses, startKeyReferenceTone } from './keyVerificationKeyboard';

describe('key verification keyboard helpers', () => {
  it('maps selected tonic and mode to their seven scale pitch classes', () => {
    expect(keyScalePitchClasses(0, 'major')).toEqual([0, 2, 4, 5, 7, 9, 11]);
    expect(keyScalePitchClasses(9, 'minor')).toEqual([9, 11, 0, 2, 4, 5, 7]);
    expect(keyScalePitchClasses(12, 'major')).toEqual([]);
    expect(keyScalePitchClasses(1.5, 'minor')).toEqual([]);
  });

  it('maps note buttons to frequencies in the middle-C octave', () => {
    expect(keyReferenceFrequency(0)).toBeCloseTo(261.6256, 2);
    expect(keyReferenceFrequency(9)).toBeCloseTo(440, 8);
    expect(() => keyReferenceFrequency(12)).toThrow(RangeError);
  });

  it('routes a short, enveloped reference tone to the isolated context and cleans up once', () => {
    const oscillator = {
      type: 'sine', frequency: { setValueAtTime: vi.fn() }, connect: vi.fn(), start: vi.fn(), stop: vi.fn(), disconnect: vi.fn(), onended: null as (() => void) | null,
    };
    const gain = {
      gain: { setValueAtTime: vi.fn(), linearRampToValueAtTime: vi.fn() }, connect: vi.fn(), disconnect: vi.fn(),
    };
    const destination = {} as AudioNode;
    const context = {
      currentTime: 3, destination,
      createOscillator: vi.fn(() => oscillator), createGain: vi.fn(() => gain),
    } as unknown as AudioContext;

    const stop = startKeyReferenceTone(context, 9);

    expect(oscillator.frequency.setValueAtTime).toHaveBeenCalledWith(440, 3);
    expect(oscillator.connect).toHaveBeenCalledWith(gain);
    expect(gain.connect).toHaveBeenCalledWith(destination);
    expect(oscillator.start).toHaveBeenCalledWith(3);
    expect(oscillator.stop).toHaveBeenCalledWith(3.52);
    expect(gain.gain.linearRampToValueAtTime).toHaveBeenCalledWith(0.08, 3.02);
    expect(gain.gain.linearRampToValueAtTime).toHaveBeenCalledWith(0, 3.52);

    stop();
    stop();
    expect(oscillator.disconnect).toHaveBeenCalledTimes(1);
    expect(gain.disconnect).toHaveBeenCalledTimes(1);
  });
});
