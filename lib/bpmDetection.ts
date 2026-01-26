/**
 * ViiB MediaHub - Client-Side BPM Detection
 * 
 * Uses Web Audio API to analyze audio and detect BPM.
 * 
 * Algorithm: Peak Detection with Autocorrelation
 * 1. Load and decode audio
 * 2. Apply low-pass filter to isolate beats
 * 3. Detect peaks (transients/onsets)
 * 4. Calculate intervals between peaks
 * 5. Find most common interval = BPM
 * 
 * @module lib/bpmDetection
 */

const ANALYSIS_SAMPLE_RATE = 22050; // Lower sample rate for faster processing
const LOW_PASS_FREQUENCY = 150; // Hz - isolate bass/kick drums
const MIN_BPM = 60;
const MAX_BPM = 200;
const ANALYSIS_DURATION = 30; // Analyze first 30 seconds

export interface BPMResult {
  bpm: number;
  confidence: number; // 0-1
  peaks: number[]; // Peak positions in seconds (for beat grid)
}

/**
 * Detects BPM from an audio URL using Web Audio API.
 * 
 * @param audioUrl - URL of the audio file to analyze
 * @param onProgress - Optional callback for progress updates
 * @returns BPM result with confidence and beat positions
 */
export async function detectBPM(
  audioUrl: string,
  onProgress?: (progress: number) => void
): Promise<BPMResult> {
  console.log(`🎵 bpmDetection: Starting analysis for ${audioUrl}`);
  
  try {
    // Fetch the audio file
    onProgress?.(0.05);
    const response = await fetch(audioUrl);
    
    if (!response.ok) {
      throw new Error(`Failed to fetch audio: ${response.status} ${response.statusText}`);
    }
    
    const arrayBuffer = await response.arrayBuffer();
    console.log(`🎵 bpmDetection: Fetched ${arrayBuffer.byteLength} bytes`);
    onProgress?.(0.15);

    // Create offline audio context for analysis
    const audioContext = new (window.OfflineAudioContext || (window as any).webkitOfflineAudioContext)(
      1, // mono
      ANALYSIS_SAMPLE_RATE * ANALYSIS_DURATION, // samples
      ANALYSIS_SAMPLE_RATE
    );
    
    // Decode the audio
    let sourceBuffer: AudioBuffer;
    try {
      // Create a normal audio context to decode first
      const tempContext = new (window.AudioContext || (window as any).webkitAudioContext)();
      sourceBuffer = await tempContext.decodeAudioData(arrayBuffer);
      tempContext.close();
      console.log(`🎵 bpmDetection: Decoded audio - ${sourceBuffer.duration.toFixed(2)}s`);
    } catch (decodeError) {
      throw new Error(`Failed to decode audio: ${decodeError}`);
    }
    onProgress?.(0.25);

    // Get a portion of the audio for analysis
    const analysisLength = Math.min(
      sourceBuffer.length,
      Math.floor(ANALYSIS_DURATION * sourceBuffer.sampleRate)
    );
    
    // Create offline context for filtered analysis
    const offlineContext = new OfflineAudioContext(
      1,
      Math.floor(analysisLength * (ANALYSIS_SAMPLE_RATE / sourceBuffer.sampleRate)),
      ANALYSIS_SAMPLE_RATE
    );

    // Create buffer source
    const source = offlineContext.createBufferSource();
    source.buffer = sourceBuffer;
    
    // Create low-pass filter to isolate bass/kicks
    const lowPass = offlineContext.createBiquadFilter();
    lowPass.type = 'lowpass';
    lowPass.frequency.value = LOW_PASS_FREQUENCY;
    lowPass.Q.value = 1.0;
    
    // Connect the graph
    source.connect(lowPass);
    lowPass.connect(offlineContext.destination);
    
    // Start playback
    source.start(0);
    
    // Render the audio
    const renderedBuffer = await offlineContext.startRendering();
    console.log(`🎵 bpmDetection: Rendered ${renderedBuffer.length} samples`);
    onProgress?.(0.5);
    
    // Get the audio data
    const audioData = renderedBuffer.getChannelData(0);
    
    // Detect peaks (onset detection)
    const peaks = detectPeaks(audioData, ANALYSIS_SAMPLE_RATE);
    console.log(`🎵 bpmDetection: Found ${peaks.length} peaks`);
    onProgress?.(0.7);
    
    // Calculate BPM from peak intervals
    const bpmResult = calculateBPMFromPeaks(peaks, ANALYSIS_SAMPLE_RATE);
    onProgress?.(0.9);
    
    // Convert peak positions to seconds
    const peakTimesSeconds = peaks.map(p => p / ANALYSIS_SAMPLE_RATE);
    
    console.log(`🎵 bpmDetection: Detected BPM = ${bpmResult.bpm.toFixed(1)}, confidence = ${(bpmResult.confidence * 100).toFixed(0)}%`);
    onProgress?.(1.0);
    
    return {
      bpm: bpmResult.bpm,
      confidence: bpmResult.confidence,
      peaks: peakTimesSeconds,
    };
    
  } catch (error) {
    console.error(`🎵 bpmDetection: Analysis failed:`, error);
    throw error;
  }
}

/**
 * Detect peaks (onset/transients) in audio data.
 * Uses adaptive threshold with energy detection.
 */
function detectPeaks(audioData: Float32Array, sampleRate: number): number[] {
  const windowSize = Math.floor(sampleRate * 0.02); // 20ms windows
  const hopSize = Math.floor(windowSize / 2);
  const peaks: number[] = [];
  
  // Calculate energy for each window
  const energies: number[] = [];
  for (let i = 0; i < audioData.length - windowSize; i += hopSize) {
    let energy = 0;
    for (let j = 0; j < windowSize; j++) {
      energy += audioData[i + j] * audioData[i + j];
    }
    energies.push(Math.sqrt(energy / windowSize));
  }
  
  // Normalize energies
  const maxEnergy = Math.max(...energies);
  if (maxEnergy > 0) {
    for (let i = 0; i < energies.length; i++) {
      energies[i] /= maxEnergy;
    }
  }
  
  // Find peaks using adaptive threshold
  const lookback = 10; // Number of windows to consider for threshold
  const thresholdMultiplier = 1.3;
  const minPeakDistance = Math.floor(sampleRate * 0.15 / hopSize); // Min 150ms between peaks
  
  let lastPeakIndex = -minPeakDistance;
  
  for (let i = lookback; i < energies.length - 1; i++) {
    // Calculate local average
    let localAvg = 0;
    for (let j = i - lookback; j < i; j++) {
      localAvg += energies[j];
    }
    localAvg /= lookback;
    
    const threshold = localAvg * thresholdMultiplier;
    
    // Check if this is a peak
    if (energies[i] > threshold && 
        energies[i] > energies[i - 1] && 
        energies[i] >= energies[i + 1] &&
        i - lastPeakIndex >= minPeakDistance) {
      peaks.push(i * hopSize);
      lastPeakIndex = i;
    }
  }
  
  return peaks;
}

/**
 * Calculate BPM from peak intervals using histogram analysis.
 */
function calculateBPMFromPeaks(peaks: number[], sampleRate: number): { bpm: number; confidence: number } {
  if (peaks.length < 4) {
    return { bpm: 120, confidence: 0 }; // Default fallback
  }
  
  // Calculate intervals between consecutive peaks
  const intervals: number[] = [];
  for (let i = 1; i < peaks.length; i++) {
    const interval = peaks[i] - peaks[i - 1];
    const bpm = (60 * sampleRate) / interval;
    
    // Filter to reasonable BPM range
    if (bpm >= MIN_BPM && bpm <= MAX_BPM) {
      intervals.push(interval);
    }
  }
  
  if (intervals.length === 0) {
    return { bpm: 120, confidence: 0 };
  }
  
  // Create histogram of intervals (grouped by BPM)
  const bpmBuckets: Map<number, number> = new Map();
  const bucketSize = 1; // 1 BPM resolution
  
  for (const interval of intervals) {
    const bpm = Math.round((60 * sampleRate) / interval);
    const bucket = Math.round(bpm / bucketSize) * bucketSize;
    bpmBuckets.set(bucket, (bpmBuckets.get(bucket) || 0) + 1);
  }
  
  // Find the most common BPM
  let maxCount = 0;
  let bestBpm = 120;
  
  bpmBuckets.forEach((count, bpm) => {
    // Also check for half-time/double-time
    const halfTimeCount = bpmBuckets.get(Math.round(bpm / 2)) || 0;
    const doubleTimeCount = bpmBuckets.get(bpm * 2) || 0;
    const totalCount = count + halfTimeCount * 0.5 + doubleTimeCount * 0.5;
    
    if (totalCount > maxCount) {
      maxCount = totalCount;
      bestBpm = bpm;
    }
  });
  
  // Calculate confidence based on how consistent the intervals are
  const expectedInterval = (60 * sampleRate) / bestBpm;
  let matchingIntervals = 0;
  const tolerance = 0.1; // 10% tolerance
  
  for (const interval of intervals) {
    const ratio = interval / expectedInterval;
    // Check if interval matches or is a multiple/divisor
    if (Math.abs(ratio - 1) < tolerance || 
        Math.abs(ratio - 2) < tolerance || 
        Math.abs(ratio - 0.5) < tolerance) {
      matchingIntervals++;
    }
  }
  
  const confidence = Math.min(1, matchingIntervals / intervals.length);
  
  return { bpm: bestBpm, confidence };
}

/**
 * Generates a beat grid based on BPM and track duration.
 * 
 * @param bpm - Detected BPM
 * @param duration - Track duration in seconds
 * @param firstBeatOffset - Time of first beat in seconds (default 0)
 * @returns Array of beat positions in seconds
 */
export function generateBeatGrid(
  bpm: number,
  duration: number,
  firstBeatOffset: number = 0
): number[] {
  const beatInterval = 60 / bpm; // seconds per beat
  const beats: number[] = [];
  
  for (let t = firstBeatOffset; t < duration; t += beatInterval) {
    beats.push(t);
  }
  
  return beats;
}

/**
 * Adjusts BPM to avoid half/double time errors.
 * If the detected BPM seems like half or double time, correct it.
 * 
 * @param bpm - Detected BPM
 * @param targetRange - Preferred BPM range [min, max]
 * @returns Adjusted BPM
 */
export function normalizeBPM(
  bpm: number,
  targetRange: [number, number] = [80, 160]
): number {
  const [min, max] = targetRange;
  
  // Double if too slow
  while (bpm < min && bpm * 2 <= MAX_BPM) {
    bpm *= 2;
  }
  
  // Halve if too fast
  while (bpm > max && bpm / 2 >= MIN_BPM) {
    bpm /= 2;
  }
  
  return bpm;
}
