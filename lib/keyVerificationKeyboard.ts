export const KEY_NOTE_NAMES = ['C', 'C♯', 'D', 'D♯', 'E', 'F', 'F♯', 'G', 'G♯', 'A', 'A♯', 'B'] as const;

export type VerifiedKeyMode = 'major' | 'minor';

const SCALE_STEPS: Record<VerifiedKeyMode, readonly number[]> = {
  major: [0, 2, 4, 5, 7, 9, 11],
  minor: [0, 2, 3, 5, 7, 8, 10],
};

export function keyScalePitchClasses(tonic: number, mode: VerifiedKeyMode): number[] {
  if (!Number.isInteger(tonic) || tonic < 0 || tonic > 11) return [];
  return SCALE_STEPS[mode].map(step => (tonic + step) % 12);
}

export function keyReferenceFrequency(pitchClass: number): number {
  if (!Number.isInteger(pitchClass) || pitchClass < 0 || pitchClass > 11) {
    throw new RangeError('Pitch class must be an integer from 0 through 11');
  }
  // Audition in the middle-C octave (MIDI 60), where C4 is 261.6256 Hz.
  return 440 * Math.pow(2, ((60 + pitchClass) - 69) / 12);
}

export function startKeyReferenceTone(context: AudioContext, pitchClass: number): () => void {
  const oscillator = context.createOscillator();
  const gain = context.createGain();
  const startAt = context.currentTime;
  const stopAt = startAt + 0.52;
  oscillator.type = 'sine';
  oscillator.frequency.setValueAtTime(keyReferenceFrequency(pitchClass), startAt);
  gain.gain.setValueAtTime(0, startAt);
  gain.gain.linearRampToValueAtTime(0.08, startAt + 0.02);
  gain.gain.linearRampToValueAtTime(0, stopAt);
  oscillator.connect(gain);
  gain.connect(context.destination);
  oscillator.start(startAt);
  oscillator.stop(stopAt);

  let stopped = false;
  const stop = () => {
    if (stopped) return;
    stopped = true;
    try { oscillator.stop(); } catch { /* The scheduled note may have ended already. */ }
    oscillator.disconnect();
    gain.disconnect();
  };
  oscillator.onended = stop;
  return stop;
}
