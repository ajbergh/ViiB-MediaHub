// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  calculateAudioBufferPeaks,
  clearWaveformCache,
  generateClientWaveform,
} from './clientWaveform';

function referenceMixAndPeaks(buffer: AudioBuffer, resolution = 1200): number[] {
  const numChannels = buffer.numberOfChannels;
  const length = buffer.length;
  const mixed = new Float32Array(length);
  for (let ch = 0; ch < numChannels; ch++) {
    const channel = buffer.getChannelData(ch);
    for (let i = 0; i < length; i++) {
      mixed[i] += channel[i] / numChannels;
    }
  }
  const peaks: number[] = [];
  const samplesPerPeak = Math.floor(mixed.length / resolution);
  let maxPeak = 0;
  for (let i = 0; i < resolution; i++) {
    const start = i * samplesPerPeak;
    const end = Math.min(start + samplesPerPeak, mixed.length);
    let sumSquares = 0;
    let maxAbs = 0;
    for (let j = start; j < end; j++) {
      const sample = mixed[j];
      sumSquares += sample * sample;
      maxAbs = Math.max(maxAbs, Math.abs(sample));
    }
    const rms = Math.sqrt(sumSquares / (end - start));
    const peak = (rms + maxAbs) / 2;
    peaks.push(peak);
    maxPeak = Math.max(maxPeak, peak);
  }
  if (maxPeak > 0) {
    for (let i = 0; i < peaks.length; i++) {
      peaks[i] = peaks[i] / maxPeak;
    }
  }
  return peaks;
}

function makeFakeAudioBuffer(channels: Float32Array[], sampleRate = 48000): AudioBuffer {
  const length = channels[0]?.length ?? 0;
  return {
    length,
    sampleRate,
    duration: length / sampleRate,
    numberOfChannels: channels.length,
    getChannelData: (ch: number) => channels[ch],
    copyFromChannel: vi.fn(),
    copyToChannel: vi.fn(),
  } as unknown as AudioBuffer;
}

describe('lib/clientWaveform', () => {
  beforeEach(() => {
    clearWaveformCache();
  });

  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it('matches reference mixChannels + calculatePeaks for mono, stereo, and 4-channel buffers without allocating Float32Arrays', () => {
    const length = 12000;
    const makeChannel = (seed: number) => {
      const arr = new Float32Array(length);
      for (let i = 0; i < length; i++) {
        arr[i] = Math.sin(i * 0.017 * seed) * (0.2 + 0.8 * ((i % 97) / 97));
      }
      return arr;
    };

    for (const chCount of [1, 2, 4]) {
      const channels = Array.from({ length: chCount }, (_, idx) => makeChannel(idx + 1));
      const buffer = makeFakeAudioBuffer(channels);
      const expected = referenceMixAndPeaks(buffer, 1200);

      const Float32Spy = vi.fn(Float32Array);
      vi.stubGlobal('Float32Array', Float32Spy);
      const actual = calculateAudioBufferPeaks(buffer, 1200);
      vi.unstubAllGlobals();

      expect(Float32Spy).not.toHaveBeenCalled();
      expect(actual).toHaveLength(1200);
      for (let i = 0; i < expected.length; i++) {
        expect(actual[i]).toBeCloseTo(expected[i], 5);
      }
    }
  });

  it('rejects immediately when already aborted before fetching', async () => {
    const fetchSpy = vi.fn();
    vi.stubGlobal('fetch', fetchSpy);
    const controller = new AbortController();
    controller.abort();

    await expect(
      generateClientWaveform('/api/audio/aborted-pre', undefined, controller.signal),
    ).rejects.toMatchObject({ name: 'AbortError' });
    expect(fetchSpy).not.toHaveBeenCalled();
  });

  it('passes AbortSignal to fetch and closes AudioContext when aborted during decodeAudioData', async () => {
    const closeSpy = vi.fn().mockResolvedValue(undefined);
    let rejectDecode!: (err: unknown) => void;
    const decodeAudioData = vi.fn(
      () => new Promise<AudioBuffer>((_resolve, reject) => {
        rejectDecode = reject;
      }),
    );

    class FakeAudioContext {
      decodeAudioData = decodeAudioData;
      close = closeSpy;
    }
    vi.stubGlobal('AudioContext', FakeAudioContext);
    const fetchSpy = vi.fn(async (_url: string, init?: RequestInit) => {
      expect(init?.signal).toBeInstanceOf(AbortSignal);
      return {
        ok: true,
        arrayBuffer: async () => new ArrayBuffer(16),
      };
    });
    vi.stubGlobal('fetch', fetchSpy);

    const controller = new AbortController();
    const promise = generateClientWaveform('/api/audio/aborted-decode', undefined, controller.signal);

    await vi.waitFor(() => expect(decodeAudioData).toHaveBeenCalledTimes(1));
    controller.abort();
    await expect(promise).rejects.toMatchObject({ name: 'AbortError' });
    expect(closeSpy).toHaveBeenCalledTimes(1);

    // Late rejection from decodeAudioData should not cause unhandled rejection
    rejectDecode(new Error('closed'));
    await Promise.resolve();
  });

  it('closes AudioContext in finally when decodeAudioData throws', async () => {
    const closeSpy = vi.fn().mockResolvedValue(undefined);
    class FakeAudioContext {
      decodeAudioData = vi.fn().mockRejectedValue(new Error('bad codec'));
      close = closeSpy;
    }
    vi.stubGlobal('AudioContext', FakeAudioContext);
    vi.stubGlobal('fetch', vi.fn(async () => ({
      ok: true,
      arrayBuffer: async () => new ArrayBuffer(8),
    })));

    await expect(generateClientWaveform('/api/audio/bad-codec')).rejects.toThrow('Failed to decode audio');
    expect(closeSpy).toHaveBeenCalledTimes(1);
  });

  it('benchmarks 6-minute stereo 48kHz peak calculation with minimal heap overhead', () => {
    const sampleRate = 48000;
    const durationSec = 360; // 6 minutes
    const length = sampleRate * durationSec; // 17,280,000 samples/channel
    const left = new Float32Array(length);
    const right = new Float32Array(length);
    // Populate representative sparse transients across the track
    for (let i = 0; i < length; i += 256) {
      left[i] = ((i % 2048) / 2048) * 0.9;
      right[i] = (((i + 512) % 2048) / 2048) * 0.85;
    }
    const buffer = makeFakeAudioBuffer([left, right], sampleRate);

    const memBefore = typeof process !== 'undefined' && typeof process.memoryUsage === 'function'
      ? process.memoryUsage().heapUsed
      : 0;
    const t0 = performance.now();
    const peaks = calculateAudioBufferPeaks(buffer, 1200);
    const elapsedMs = performance.now() - t0;
    const memAfter = typeof process !== 'undefined' && typeof process.memoryUsage === 'function'
      ? process.memoryUsage().heapUsed
      : 0;
    const heapDeltaBytes = memAfter - memBefore;

    console.info(
      `[benchmark] 6-min stereo 48kHz (${length.toLocaleString()} samples/ch): ${elapsedMs.toFixed(2)}ms, heapDelta=${heapDeltaBytes.toLocaleString()} bytes`,
    );

    expect(peaks).toHaveLength(1200);
    expect(Math.max(...peaks)).toBeCloseTo(1, 5);
    expect(Math.min(...peaks)).toBeGreaterThanOrEqual(0);
    expect(elapsedMs).toBeLessThan(2000);
    // Old mixChannels allocated ~69MB Float32Array; zero-copy path should allocate negligible JS heap (< 2MB)
    expect(heapDeltaBytes).toBeLessThan(2 * 1024 * 1024);
  });
});
