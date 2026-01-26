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

/**
 * Generates waveform peak data from an audio URL using Web Audio API.
 * 
 * @param audioUrl - URL of the audio file to analyze
 * @param onProgress - Optional callback for progress updates
 * @returns Array of normalized peak values (0-1)
 */
export async function generateClientWaveform(
  audioUrl: string,
  onProgress?: (progress: number) => void
): Promise<number[]> {
  console.log(`🎵 clientWaveform: Starting generation for ${audioUrl}`);
  
  try {
    // Fetch the audio file
    onProgress?.(0.1);
    const response = await fetch(audioUrl);
    
    if (!response.ok) {
      throw new Error(`Failed to fetch audio: ${response.status} ${response.statusText}`);
    }
    
    const arrayBuffer = await response.arrayBuffer();
    console.log(`🎵 clientWaveform: Fetched ${arrayBuffer.byteLength} bytes`);
    onProgress?.(0.3);

    // Decode the audio
    const audioContext = new (window.AudioContext || (window as any).webkitAudioContext)();
    
    let audioBuffer: AudioBuffer;
    try {
      audioBuffer = await audioContext.decodeAudioData(arrayBuffer);
      console.log(`🎵 clientWaveform: Decoded audio - ${audioBuffer.duration.toFixed(2)}s, ${audioBuffer.numberOfChannels} channels, ${audioBuffer.sampleRate}Hz`);
    } catch (decodeError) {
      audioContext.close();
      throw new Error(`Failed to decode audio: ${decodeError}`);
    }
    onProgress?.(0.5);

    // Get channel data (use first channel, or mix if stereo)
    const channelData = audioBuffer.numberOfChannels > 1
      ? mixChannels(audioBuffer)
      : audioBuffer.getChannelData(0);
    
    // Generate peaks
    const peaks = calculatePeaks(channelData, WAVEFORM_RESOLUTION);
    onProgress?.(0.9);

    // Clean up
    audioContext.close();
    
    console.log(`🎵 clientWaveform: Generated ${peaks.length} peaks`);
    onProgress?.(1.0);
    
    return peaks;
    
  } catch (error) {
    console.error(`🎵 clientWaveform: Generation failed:`, error);
    throw error;
  }
}

/**
 * Mixes multiple channels down to mono.
 */
function mixChannels(buffer: AudioBuffer): Float32Array {
  const numChannels = buffer.numberOfChannels;
  const length = buffer.length;
  const mixed = new Float32Array(length);
  
  for (let ch = 0; ch < numChannels; ch++) {
    const channel = buffer.getChannelData(ch);
    for (let i = 0; i < length; i++) {
      mixed[i] += channel[i] / numChannels;
    }
  }
  
  return mixed;
}

/**
 * Calculates peak values from audio samples.
 * Uses RMS (Root Mean Square) for smoother visualization.
 */
function calculatePeaks(samples: Float32Array, resolution: number): number[] {
  const peaks: number[] = [];
  const samplesPerPeak = Math.floor(samples.length / resolution);
  
  let maxPeak = 0;
  
  // First pass: calculate raw peaks
  for (let i = 0; i < resolution; i++) {
    const start = i * samplesPerPeak;
    const end = Math.min(start + samplesPerPeak, samples.length);
    
    // Calculate RMS for this segment
    let sumSquares = 0;
    let maxAbs = 0;
    
    for (let j = start; j < end; j++) {
      const sample = samples[j];
      sumSquares += sample * sample;
      maxAbs = Math.max(maxAbs, Math.abs(sample));
    }
    
    // Use a combination of RMS and peak for better visualization
    const rms = Math.sqrt(sumSquares / (end - start));
    const peak = (rms + maxAbs) / 2;
    
    peaks.push(peak);
    maxPeak = Math.max(maxPeak, peak);
  }
  
  // Normalize to 0-1 range
  if (maxPeak > 0) {
    for (let i = 0; i < peaks.length; i++) {
      peaks[i] = peaks[i] / maxPeak;
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
