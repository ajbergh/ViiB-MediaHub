/** Deterministic ViiB-owned colors for the twelve Camelot wheel positions. */
export const CAMELOT_BASE_HUES = {
  1: '#49D3C5',
  2: '#67CE7B',
  3: '#A8CF62',
  4: '#E8CF5B',
  5: '#F2B65A',
  6: '#F38B83',
  7: '#EE6F9D',
  8: '#DA63C1',
  9: '#B66FE1',
  10: '#8B82E3',
  11: '#6EA6DF',
  12: '#50C5DF',
} as const;

export type CamelotMode = 'A' | 'B';

export interface CamelotColor {
  /** Shared hue for the Camelot wheel number, as specified by the ViiB palette. */
  hue: string;
  /** Mode-specific tone, keeping A and B in the same hue family. */
  background: string;
  border: string;
  foreground: string;
}

function blendHex(hex: string, target: number, amount: number): string {
  const channels = [1, 3, 5].map(offset => parseInt(hex.slice(offset, offset + 2), 16));
  const blended = channels.map(channel => Math.round(channel + (target - channel) * amount));
  return `#${blended.map(channel => channel.toString(16).padStart(2, '0')).join('')}`.toUpperCase();
}

/** Return a stable color treatment for a valid Camelot code; null for unknown keys. */
export function getCamelotColor(code: string | null | undefined): CamelotColor | null {
  const normalized = code?.trim().toUpperCase().match(/^(1[0-2]|[1-9])([AB])$/);
  if (!normalized) return null;

  const number = Number(normalized[1]) as keyof typeof CAMELOT_BASE_HUES;
  const mode = normalized[2] as CamelotMode;
  const hue = CAMELOT_BASE_HUES[number];

  return {
    hue,
    // Minor (A) gets a deeper tone; major (B) a lighter tone. The wheel-position
    // hue remains the same for both modes.
    background: mode === 'A' ? blendHex(hue, 0, 0.48) : blendHex(hue, 255, 0.30),
    border: mode === 'A' ? hue : blendHex(hue, 0, 0.12),
    foreground: mode === 'A' ? '#F4F6F8' : '#101316',
  };
}
