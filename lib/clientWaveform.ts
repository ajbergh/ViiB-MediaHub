/**
 * ViiB MediaHub - Client-Side Waveform Generation
 * 
 * Uses Web Audio API to generate waveform peaks for audio files
 * that aren't supported by the server-side decoder.
 * 
 * Supports all formats the browser can decode:
 * - OGG Vorbis/Opus
 * - FLAC
 * - WAV
 * - AAC/M4A
 * - MP3
 * 
 * @module lib/clientWaveform
 */

const WAVEFORM_RESOLUTION = 1200; // Number of peaks to generate
const OVERVIEW_RESOLUTION = 200;  // Downsampled for overview strip (saves memory/CPU)
const WAVEFORM_CACHE_MAX = 10;    // Max cached waveforms (oldest-insertion eviction)
const PEAK_SAMPLE_CHUNK_SIZE = 262_144;
const PEAK_FRAME_BUDGET_MS = 6;

// Bounded insertion-ordered waveform cache to avoid regenerating peaks for previously loaded tracks
const waveformCache = new Map<string, { peaks: number[]; overview: number[] }>();

function createAbortError(signal?: AbortSignal): Error {
  const reason = signal?.reason;
  if (reason instanceof Error) return reason;
  if (typeof DOMException !== 'undefined') {
    if (reason instanceof DOMException) return reason;
    return new DOMException(typeof reason === 'string' ? reason : 'The operation was aborted.', 'AbortError');
  }
  const err = new Error(typeof reason === 'string' ? reason : 'The operation was aborted.');
  err.name = 'AbortError';
  return err;
}

function throwIfAborted(signal?: AbortSignal): void {
  if (signal?.aborted) {
    throw createAbortError(signal);
  }
}

function isAbortError(error: unknown): boolean {
  return (error as { name?: string } | null)?.name === 'AbortError';
}

/**
 * Get cached waveform data for a URL if available.
 */
export function getCachedWaveform(audioUrl: string): { peaks: number[]; overview: number[] } | null {
  return waveformCache.get(audioUrl) || null;
}

/**
 * Clear the in-memory client waveform cache (primarily for tests).
 */
export function clearWaveformCache(): void {
  waveformCache.clear();
}

/**
 * Downsample a peaks array to a lower resolution for overview display.
 */
function downsamplePeaks(peaks: number[], targetResolution: number): number[] {
  if (peaks.length <= targetResolution) return peaks;
  
  const result: number[] = [];
  const samplesPerBin = peaks.length / targetResolution;
  
  for (let i = 0; i < targetResolution; i++) {
    const start = Math.floor(i * samplesPerBin);
    const end = Math.floor((i + 1) * samplesPerBin);
    let max = 0;
    for (let j = start; j < end; j++) {
      if (peaks[j] > max) max = peaks[j];
    }
    result.push(max);
  }
  
  return result;
}

/**
 * Generates waveform peak data from an audio URL using Web Audio API.
 * 
 * @param audioUrl - URL of the audio file to analyze
 * @param onProgress - Optional callback for progress updates
 * @param signal - Optional AbortSignal to cancel fetching, decoding, or peak calculation
 * @returns Array of normalized peak values (0-1)
 */
export async function generateClientWaveform(
  audioUrl: string,
  onProgress?: (progress: number) => void,
  signal?: AbortSignal
): Promise<number[]> {
  throwIfAborted(signal);
  console.log(`🎵 clientWaveform: Starting generation for ${audioUrl}`);
  
  // Check cache first
  const cached = waveformCache.get(audioUrl);
  if (cached) {
    console.log(`🎵 clientWaveform: Cache hit for ${audioUrl} (${cached.peaks.length} peaks)`);
    onProgress?.(1.0);
    return cached.peaks;
  }
  
  try {
    // Fetch the audio file
    onProgress?.(0.1);
    const response = await fetch(audioUrl, { signal });
    
    if (!response.ok) {
      throw new Error(`Failed to fetch audio: ${response.status} ${response.statusText}`);
    }
    
    const arrayBuffer = await response.arrayBuffer();
    throwIfAborted(signal);
    console.log(`🎵 clientWaveform: Fetched ${arrayBuffer.byteLength} bytes`);
    onProgress?.(0.3);

    // Decode the audio and always close the context when done or aborted
    const AudioContextCtor = window.AudioContext || (window as typeof window & { webkitAudioContext?: typeof AudioContext }).webkitAudioContext!;
    const audioContext = new AudioContextCtor();
    let peaks: number[];

    try {
      throwIfAborted(signal);
      let audioBuffer: AudioBuffer;
      const decodePromise = audioContext.decodeAudioData(arrayBuffer);

      if (signal) {
        audioBuffer = await new Promise<AudioBuffer>((resolve, reject) => {
          const onAbort = () => reject(createAbortError(signal));
          if (signal.aborted) {
            onAbort();
            return;
          }
          signal.addEventListener('abort', onAbort, { once: true });
          decodePromise.then(
            (decoded) => {
              signal.removeEventListener('abort', onAbort);
              resolve(decoded);
            },
            (decodeError) => {
              signal.removeEventListener('abort', onAbort);
              reject(new Error(`Failed to decode audio: ${decodeError}`));
            },
          );
        });
      } else {
        try {
          audioBuffer = await decodePromise;
        } catch (decodeError) {
          throw new Error(`Failed to decode audio: ${decodeError}`);
        }
      }

      throwIfAborted(signal);
      console.log(`🎵 clientWaveform: Decoded audio - ${audioBuffer.duration.toFixed(2)}s, ${audioBuffer.numberOfChannels} channels, ${audioBuffer.sampleRate}Hz`);
      onProgress?.(0.5);

      peaks = await calculateAudioBufferPeaksAsync(audioBuffer, WAVEFORM_RESOLUTION, signal);
      throwIfAborted(signal);
      onProgress?.(0.9);
    } finally {
      await Promise.resolve(audioContext.close()).catch(() => {});
    }
    
    console.log(`🎵 clientWaveform: Generated ${peaks.length} peaks`);
    onProgress?.(1.0);
    
    // Cache the results (with oldest-insertion eviction)
    const overview = downsamplePeaks(peaks, OVERVIEW_RESOLUTION);
    if (waveformCache.size >= WAVEFORM_CACHE_MAX) {
      // Evict oldest entry (first key in Map)
      const firstKey = waveformCache.keys().next().value;
      if (firstKey) waveformCache.delete(firstKey);
    }
    waveformCache.set(audioUrl, { peaks, overview });
    
    return peaks;
    
  } catch (error) {
    if (!isAbortError(error)) {
      console.error(`🎵 clientWaveform: Generation failed:`, error);
    }
    throw error;
  }
}

/**
 * Calculates normalized peak values directly from an AudioBuffer's channels
 * in bounded chunks without allocating a full-length mono Float32Array.
 * Uses a combination of RMS and peak amplitude for visualization.
 */
export function calculateAudioBufferPeaks(
  audioBuffer: AudioBuffer,
  resolution: number = WAVEFORM_RESOLUTION,
  signal?: AbortSignal
): number[] {
  const work = calculatePeakChunks(audioBuffer, resolution);
  for (;;) {
    throwIfAborted(signal);
    const step = work.next();
    if (step.done) return step.value;
  }
}

export async function calculateAudioBufferPeaksAsync(
  audioBuffer: AudioBuffer,
  resolution: number = WAVEFORM_RESOLUTION,
  signal?: AbortSignal,
): Promise<number[]> {
  const work = calculatePeakChunks(audioBuffer, resolution);
  let deadline = performance.now() + PEAK_FRAME_BUDGET_MS;
  for (;;) {
    throwIfAborted(signal);
    const step = work.next();
    if (step.done) return step.value;
    if (performance.now() >= deadline) {
      await new Promise<void>(resolve => setTimeout(resolve, 0));
      deadline = performance.now() + PEAK_FRAME_BUDGET_MS;
    }
  }
}

function* calculatePeakChunks(audioBuffer: AudioBuffer, resolution: number): Generator<void, number[]> {
  const numChannels = audioBuffer.numberOfChannels;
  const length = audioBuffer.length;
  const peaks = new Array<number>(resolution);

  if (numChannels <= 0 || length <= 0 || resolution <= 0) {
    return peaks.fill(0);
  }

  const channels: Float32Array[] = new Array(numChannels);
  for (let ch = 0; ch < numChannels; ch++) {
    channels[ch] = audioBuffer.getChannelData(ch);
  }

  const samplesPerPeak = Math.floor(length / resolution);
  let maxPeak = 0;

  if (numChannels === 1) {
    const ch0 = channels[0];
    for (let i = 0; i < resolution; i++) {
      const start = i * samplesPerPeak;
      const end = Math.min(start + samplesPerPeak, length);
      const count = end - start;
      if (count <= 0) {
        peaks[i] = 0;
        continue;
      }
      let sumSquares = 0;
      let maxAbs = 0;
      for (let j = start; j < end;) {
        const chunkEnd = Math.min(j + PEAK_SAMPLE_CHUNK_SIZE, end);
        for (; j < chunkEnd; j++) {
          const sample = ch0[j];
          sumSquares += sample * sample;
          const abs = sample < 0 ? -sample : sample;
          if (abs > maxAbs) maxAbs = abs;
        }
        yield;
      }
      const rms = Math.sqrt(sumSquares / count);
      const peak = (rms + maxAbs) * 0.5;
      peaks[i] = peak;
      if (peak > maxPeak) maxPeak = peak;
    }
  } else if (numChannels === 2) {
    const ch0 = channels[0];
    const ch1 = channels[1];
    for (let i = 0; i < resolution; i++) {
      const start = i * samplesPerPeak;
      const end = Math.min(start + samplesPerPeak, length);
      const count = end - start;
      if (count <= 0) {
        peaks[i] = 0;
        continue;
      }
      let sumSquares = 0;
      let maxAbs = 0;
      for (let j = start; j < end;) {
        const chunkEnd = Math.min(j + PEAK_SAMPLE_CHUNK_SIZE / 2, end);
        for (; j < chunkEnd; j++) {
          const sample = (ch0[j] + ch1[j]) * 0.5;
          sumSquares += sample * sample;
          const abs = sample < 0 ? -sample : sample;
          if (abs > maxAbs) maxAbs = abs;
        }
        yield;
      }
      const rms = Math.sqrt(sumSquares / count);
      const peak = (rms + maxAbs) * 0.5;
      peaks[i] = peak;
      if (peak > maxPeak) maxPeak = peak;
    }
  } else {
    const invChannels = 1 / numChannels;
    const chunkSize = Math.max(1, Math.floor(PEAK_SAMPLE_CHUNK_SIZE / numChannels));
    for (let i = 0; i < resolution; i++) {
      const start = i * samplesPerPeak;
      const end = Math.min(start + samplesPerPeak, length);
      const count = end - start;
      if (count <= 0) {
        peaks[i] = 0;
        continue;
      }
      let sumSquares = 0;
      let maxAbs = 0;
      for (let j = start; j < end;) {
        const chunkEnd = Math.min(j + chunkSize, end);
        for (; j < chunkEnd; j++) {
          let sample = 0;
          for (let ch = 0; ch < numChannels; ch++) {
            sample += channels[ch][j];
          }
          sample *= invChannels;
          sumSquares += sample * sample;
          const abs = sample < 0 ? -sample : sample;
          if (abs > maxAbs) maxAbs = abs;
        }
        yield;
      }
      const rms = Math.sqrt(sumSquares / count);
      const peak = (rms + maxAbs) * 0.5;
      peaks[i] = peak;
      if (peak > maxPeak) maxPeak = peak;
    }
  }

  // Normalize to 0-1 range
  if (maxPeak > 0) {
    const invMax = 1 / maxPeak;
    for (let i = 0; i < peaks.length; i++) {
      peaks[i] *= invMax;
    }
  }

  return peaks;
}

/**
 * Checks if the browser supports a given audio format for decoding.
 */
export function canDecodeFormat(mimeType: string): boolean {
  const audio = document.createElement('audio');
  return audio.canPlayType(mimeType) !== '';
}

/**
 * Gets the MIME type for a file extension.
 */
export function getMimeType(filePath: string): string {
  const ext = filePath.toLowerCase().split('.').pop();
  
  const mimeTypes: Record<string, string> = {
    'mp3': 'audio/mpeg',
    'ogg': 'audio/ogg',
    'oga': 'audio/ogg',
    'opus': 'audio/opus',
    'flac': 'audio/flac',
    'wav': 'audio/wav',
    'wave': 'audio/wav',
    'm4a': 'audio/mp4',
    'aac': 'audio/aac',
  };
  
  return mimeTypes[ext || ''] || 'audio/mpeg';
}
