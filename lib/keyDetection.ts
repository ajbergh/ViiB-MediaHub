/**
 * ViiB MediaHub - Musical Key Detection
 * 
 * Detects the musical key of audio using chromagram analysis and
 * the Krumhansl-Schmuckler algorithm for key profile matching.
 * 
 * Algorithm:
 * 1. Compute FFT to get frequency spectrum
 * 2. Map frequencies to pitch classes (chromagram)
 * 3. Compare chromagram with key profiles
 * 4. Return best matching key
 * 
 * @module lib/keyDetection
 */

// Key profiles from Krumhansl-Schmuckler (normalized probe tone ratings)
// Index 0 = C, 1 = C#/Db, 2 = D, ..., 11 = B
const MAJOR_PROFILE = [
  6.35, 2.23, 3.48, 2.33, 4.38, 4.09, 2.52, 5.19, 2.39, 3.66, 2.29, 2.88
];

const MINOR_PROFILE = [
  6.33, 2.68, 3.52, 5.38, 2.60, 3.53, 2.54, 4.75, 3.98, 2.69, 3.34, 3.17
];

// Pitch class names for key labeling
const PITCH_CLASSES = ['C', 'C#', 'D', 'D#', 'E', 'F', 'F#', 'G', 'G#', 'A', 'A#', 'B'];
const PITCH_CLASSES_FLAT = ['C', 'Db', 'D', 'Eb', 'E', 'F', 'Gb', 'G', 'Ab', 'A', 'Bb', 'B'];

// Camelot wheel notation (for mixing compatibility)
const CAMELOT_WHEEL: Record<string, string> = {
  'C major': '8B', 'G major': '9B', 'D major': '10B', 'A major': '11B',
  'E major': '12B', 'B major': '1B', 'F# major': '2B', 'C# major': '3B',
  'Ab major': '4B', 'Eb major': '5B', 'Bb major': '6B', 'F major': '7B',
  'A minor': '8A', 'E minor': '9A', 'B minor': '10A', 'F# minor': '11A',
  'C# minor': '12A', 'G# minor': '1A', 'D# minor': '2A', 'A# minor': '3A',
  'F minor': '4A', 'C minor': '5A', 'G minor': '6A', 'D minor': '7A',
};

// Open Key notation (alternative to Camelot)
const OPEN_KEY: Record<string, string> = {
  'C major': '1d', 'G major': '2d', 'D major': '3d', 'A major': '4d',
  'E major': '5d', 'B major': '6d', 'F# major': '7d', 'Db major': '8d',
  'Ab major': '9d', 'Eb major': '10d', 'Bb major': '11d', 'F major': '12d',
  'A minor': '1m', 'E minor': '2m', 'B minor': '3m', 'F# minor': '4m',
  'C# minor': '5m', 'G# minor': '6m', 'Eb minor': '7m', 'Bb minor': '8m',
  'F minor': '9m', 'C minor': '10m', 'G minor': '11m', 'D minor': '12m',
};

export interface KeyDetectionResult {
  /** Musical key (e.g., "Am", "C") */
  key: string;
  /** Full key name (e.g., "A minor", "C major") */
  keyFull: string;
  /** Whether key is minor */
  isMinor: boolean;
  /** Tonic pitch class (0=C, 1=C#, ..., 11=B) */
  tonic: number;
  /** Correlation score (0-1, higher = more confident) */
  confidence: number;
  /** Camelot wheel notation for DJ mixing */
  camelot: string;
  /** Open Key notation */
  openKey: string;
}

/**
 * Detect the musical key of an audio file
 * @param audioUrl URL of the audio file
 * @param options Detection options
 * @returns Key detection result
 */
export async function detectKey(
  audioUrl: string,
  options: {
    /** Duration to analyze in seconds (default: full track) */
    duration?: number;
    /** Start offset in seconds (default: 0) */
    offset?: number;
  } = {}
): Promise<KeyDetectionResult> {
  const { duration, offset = 0 } = options;

  // Create audio context for analysis
  const audioContext = new (window.AudioContext || (window as any).webkitAudioContext)();
  
  try {
    // Fetch and decode audio
    const response = await fetch(audioUrl);
    const arrayBuffer = await response.arrayBuffer();
    const audioBuffer = await audioContext.decodeAudioData(arrayBuffer);
    
    // Get mono channel data
    const channelData = audioBuffer.getChannelData(0);
    const sampleRate = audioBuffer.sampleRate;
    
    // Calculate sample range
    const startSample = Math.floor(offset * sampleRate);
    const endSample = duration 
      ? Math.min(startSample + Math.floor(duration * sampleRate), channelData.length)
      : channelData.length;
    
    // Extract samples for analysis
    const samples = channelData.slice(startSample, endSample);
    
    // Compute chromagram
    const chromagram = computeChromagram(samples, sampleRate);
    
    // Find best matching key
    const result = findKey(chromagram);
    
    return result;
  } finally {
    await audioContext.close();
  }
}

/**
 * Compute chromagram (pitch class profile) from audio samples
 */
function computeChromagram(samples: Float32Array, sampleRate: number): number[] {
  // FFT size - larger = better frequency resolution but more computation
  const fftSize = 8192;
  const hopSize = fftSize / 4;
  
  // Initialize chromagram (12 pitch classes)
  const chromagram = new Array(12).fill(0);
  let frameCount = 0;
  
  // Hann window function
  const window = new Float32Array(fftSize);
  for (let i = 0; i < fftSize; i++) {
    window[i] = 0.5 * (1 - Math.cos(2 * Math.PI * i / fftSize));
  }
  
  // Process audio in frames
  for (let i = 0; i + fftSize <= samples.length; i += hopSize) {
    // Extract windowed frame
    const frame = new Float32Array(fftSize);
    for (let j = 0; j < fftSize; j++) {
      frame[j] = samples[i + j] * window[j];
    }
    
    // Compute FFT magnitude spectrum
    const spectrum = computeFFTMagnitude(frame);
    
    // Map spectrum to chromagram
    accumulateChroma(spectrum, sampleRate, fftSize, chromagram);
    frameCount++;
  }
  
  // Normalize chromagram
  if (frameCount > 0) {
    const max = Math.max(...chromagram);
    if (max > 0) {
      for (let i = 0; i < 12; i++) {
        chromagram[i] /= max;
      }
    }
  }
  
  return chromagram;
}

/**
 * Simple FFT implementation (Cooley-Tukey radix-2)
 * Returns magnitude spectrum
 */
function computeFFTMagnitude(samples: Float32Array): number[] {
  const n = samples.length;
  
  // Bit reversal
  const real = new Float32Array(n);
  const imag = new Float32Array(n);
  
  for (let i = 0; i < n; i++) {
    let j = 0;
    let x = i;
    for (let k = 1; k < n; k <<= 1) {
      j = (j << 1) | (x & 1);
      x >>= 1;
    }
    real[j] = samples[i];
  }
  
  // FFT butterfly
  for (let size = 2; size <= n; size *= 2) {
    const halfSize = size / 2;
    const step = Math.PI / halfSize;
    
    for (let i = 0; i < n; i += size) {
      let angle = 0;
      for (let j = i; j < i + halfSize; j++) {
        const cos = Math.cos(angle);
        const sin = -Math.sin(angle);
        
        const tReal = real[j + halfSize] * cos - imag[j + halfSize] * sin;
        const tImag = real[j + halfSize] * sin + imag[j + halfSize] * cos;
        
        real[j + halfSize] = real[j] - tReal;
        imag[j + halfSize] = imag[j] - tImag;
        real[j] += tReal;
        imag[j] += tImag;
        
        angle += step;
      }
    }
  }
  
  // Compute magnitude spectrum (only need first half)
  const magnitude = new Array(n / 2);
  for (let i = 0; i < n / 2; i++) {
    magnitude[i] = Math.sqrt(real[i] * real[i] + imag[i] * imag[i]);
  }
  
  return magnitude;
}

/**
 * Map frequency spectrum bins to chromagram
 */
function accumulateChroma(
  spectrum: number[], 
  sampleRate: number, 
  fftSize: number,
  chromagram: number[]
): void {
  const binFrequency = sampleRate / fftSize;
  
  // Analyze frequency range: 65 Hz (C2) to 2093 Hz (C7)
  const minFreq = 65;
  const maxFreq = 2093;
  
  for (let bin = 0; bin < spectrum.length; bin++) {
    const freq = bin * binFrequency;
    
    if (freq >= minFreq && freq <= maxFreq && spectrum[bin] > 0.001) {
      // Convert frequency to pitch class
      // Formula: pitch class = 12 * log2(freq / C0) mod 12
      // where C0 ≈ 16.35 Hz
      const pitchClass = Math.round(12 * Math.log2(freq / 16.35)) % 12;
      
      // Weight by magnitude (squared for emphasis on loud partials)
      const normalizedPitch = ((pitchClass % 12) + 12) % 12;
      chromagram[normalizedPitch] += spectrum[bin] * spectrum[bin];
    }
  }
}

/**
 * Find the best matching key using Krumhansl-Schmuckler algorithm
 */
function findKey(chromagram: number[]): KeyDetectionResult {
  let bestKey = 0;
  let bestMode: 'major' | 'minor' = 'major';
  let bestCorrelation = -Infinity;
  
  // Try all 12 major and 12 minor keys
  for (let tonic = 0; tonic < 12; tonic++) {
    // Rotate profiles to align with tonic
    const majorCorr = correlate(chromagram, rotateProfile(MAJOR_PROFILE, tonic));
    const minorCorr = correlate(chromagram, rotateProfile(MINOR_PROFILE, tonic));
    
    if (majorCorr > bestCorrelation) {
      bestCorrelation = majorCorr;
      bestKey = tonic;
      bestMode = 'major';
    }
    
    if (minorCorr > bestCorrelation) {
      bestCorrelation = minorCorr;
      bestKey = tonic;
      bestMode = 'minor';
    }
  }
  
  // Normalize correlation to 0-1 confidence
  const confidence = Math.max(0, Math.min(1, (bestCorrelation + 1) / 2));
  
  // Format key name
  const useFlat = bestMode === 'minor' && [1, 3, 6, 8, 10].includes(bestKey);
  const pitchName = useFlat ? PITCH_CLASSES_FLAT[bestKey] : PITCH_CLASSES[bestKey];
  
  const keyFull = `${pitchName} ${bestMode}`;
  const key = bestMode === 'minor' ? `${pitchName}m` : pitchName;
  
  return {
    key,
    keyFull,
    isMinor: bestMode === 'minor',
    tonic: bestKey,
    confidence,
    camelot: CAMELOT_WHEEL[keyFull] || 'N/A',
    openKey: OPEN_KEY[keyFull] || 'N/A',
  };
}

/**
 * Rotate a key profile to start at a different pitch class
 */
function rotateProfile(profile: number[], shift: number): number[] {
  const result = new Array(12);
  for (let i = 0; i < 12; i++) {
    result[i] = profile[(i - shift + 12) % 12];
  }
  return result;
}

/**
 * Compute Pearson correlation coefficient between two arrays
 */
function correlate(a: number[], b: number[]): number {
  const n = a.length;
  
  // Compute means
  let sumA = 0, sumB = 0;
  for (let i = 0; i < n; i++) {
    sumA += a[i];
    sumB += b[i];
  }
  const meanA = sumA / n;
  const meanB = sumB / n;
  
  // Compute correlation
  let numerator = 0;
  let denomA = 0;
  let denomB = 0;
  
  for (let i = 0; i < n; i++) {
    const diffA = a[i] - meanA;
    const diffB = b[i] - meanB;
    numerator += diffA * diffB;
    denomA += diffA * diffA;
    denomB += diffB * diffB;
  }
  
  const denominator = Math.sqrt(denomA * denomB);
  return denominator > 0 ? numerator / denominator : 0;
}

/**
 * Check if two keys are harmonically compatible
 * Returns compatibility score (0-1)
 */
export function getKeyCompatibility(key1: string, key2: string): number {
  const camelot1 = CAMELOT_WHEEL[key1];
  const camelot2 = CAMELOT_WHEEL[key2];
  
  if (!camelot1 || !camelot2) return 0.5; // Unknown key
  
  const number1 = parseInt(camelot1);
  const number2 = parseInt(camelot2);
  const letter1 = camelot1.slice(-1);
  const letter2 = camelot2.slice(-1);
  
  // Same key = perfect match
  if (camelot1 === camelot2) return 1.0;
  
  // Adjacent on wheel (±1) same letter = good match
  const diff = Math.abs(number1 - number2);
  const numberMatch = diff === 1 || diff === 11; // 11 accounts for 12→1 wrap
  
  if (numberMatch && letter1 === letter2) return 0.9;
  
  // Same number, different letter (relative major/minor) = good match
  if (number1 === number2 && letter1 !== letter2) return 0.85;
  
  // Adjacent number, different letter = decent match
  if (numberMatch && letter1 !== letter2) return 0.7;
  
  // ±2 on wheel = okay match
  const diff2 = diff === 2 || diff === 10;
  if (diff2) return 0.5;
  
  // Everything else = poor match
  return 0.3;
}

/**
 * Get harmonic mixing suggestions based on current key
 */
export function getHarmonicSuggestions(key: string): string[] {
  const camelot = CAMELOT_WHEEL[key];
  if (!camelot) return [];
  
  const number = parseInt(camelot);
  const letter = camelot.slice(-1);
  
  const suggestions: string[] = [];
  
  // Same key
  suggestions.push(key);
  
  // Find keys with adjacent Camelot numbers
  for (const [keyName, cam] of Object.entries(CAMELOT_WHEEL)) {
    const camNum = parseInt(cam);
    const camLetter = cam.slice(-1);
    
    const diff = Math.abs(number - camNum);
    const isAdjacent = diff === 1 || diff === 11;
    
    // Adjacent same letter
    if (isAdjacent && camLetter === letter && keyName !== key) {
      suggestions.push(keyName);
    }
    
    // Same number different letter (relative major/minor)
    if (camNum === number && camLetter !== letter) {
      suggestions.push(keyName);
    }
  }
  
  return suggestions;
}
