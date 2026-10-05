/** Builds reverb impulses and translates beat fractions into delay timing. */

import type { BeatFraction } from '../slices/djMixerSlice';

export function createReverbImpulse(
    ctx: AudioContext,
    convolver: ConvolverNode,
    roomSize: number,
    damping: number
): void {
    const sampleRate = ctx.sampleRate;
    const length = sampleRate * (0.5 + roomSize * 2.5); // 0.5-3 seconds
    const impulse = ctx.createBuffer(2, length, sampleRate);

    for (let channel = 0; channel < 2; channel++) {
      const channelData = impulse.getChannelData(channel);
      for (let i = 0; i < length; i++) {
        // Exponential decay with noise
        const decay = Math.pow(1 - damping, i / sampleRate * 10);
        channelData[i] = (Math.random() * 2 - 1) * decay;
      }
    }

    convolver.buffer = impulse;
  }


export function beatFractionToMultiplier(fraction: BeatFraction): number {
    switch (fraction) {
      case '1/4':
        return 0.25;
      case '1/2':
        return 0.5;
      case '2':
        return 2;
      case '4':
        return 4;
      case '1':
      default:
        return 1;
    }
  }

export function getBeatFXDelayTime(fraction: BeatFraction, bpm: number): number {
    const safeBpm = (typeof bpm === 'number' && isFinite(bpm))
      ? Math.max(40, Math.min(240, bpm))
      : 120;
    const beatSeconds = 60 / safeBpm;
    return Math.max(0.01, Math.min(2, beatSeconds * beatFractionToMultiplier(fraction)));
  }
