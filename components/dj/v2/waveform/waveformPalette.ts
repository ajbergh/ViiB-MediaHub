/**
 * Shared DJv2 waveform palette.
 *
 * Canvas and WebGL renderers read colors from here instead of keeping their
 * own copies. Values mirror the DJ tokens in index.css; the render loops use
 * these constants because reading CSS variables per frame is too costly.
 *
 * @module components/dj/v2/waveform/waveformPalette
 */

import type { DeckId } from '../../../../slices/djMixerSlice';

export type WaveformColorMode = 'rgb' | '3band' | 'single';

export const WAVEFORM_COLOR_MODES: ReadonlyArray<{ mode: WaveformColorMode; label: string; title: string }> = [
  { mode: 'rgb', label: 'GRAD', title: 'Waveform: deck spectral gradient' },
  { mode: '3band', label: 'LEVEL', title: 'Waveform: amplitude level' },
  { mode: 'single', label: 'SOLID', title: 'Waveform: solid deck color' },
];

/** WebGL shaders index color modes numerically. */
export const WEBGL_COLOR_MODE: Record<WaveformColorMode, 0 | 1 | 2> = { rgb: 0, '3band': 1, single: 2 };

export interface DeckWaveformPalette {
  /** Deck identity color. */
  deck: string;
  /** Bright deck color for playhead-adjacent accents in the overview. */
  bright: string;
  /** Translucent overview fill. */
  overview: string;
  /** Gradient-mode stops from the center line out to the waveform edge. */
  gradient: { center: string; mid: string; edge: string };
}

export const DECK_WAVEFORM_PALETTE: Record<DeckId, DeckWaveformPalette> = {
  // Deck A: cool spectral progression (blue → cyan → green tips).
  A: {
    deck: '#0868f8',
    bright: '#2088f8',
    overview: '#2088f88c',
    gradient: { center: '#1d6bff', mid: '#18c4ff', edge: '#3fe28a' },
  },
  // Deck B: warm/violet progression (violet → magenta → orange tips).
  B: {
    deck: '#8030f8',
    bright: '#9d5cff',
    overview: '#9d5cff8c',
    gradient: { center: '#7a2cff', mid: '#e040ff', edge: '#ff8a3d' },
  },
};

/** Symmetric top-to-bottom gradient stops for Canvas 2D (edge → center → edge). */
export function canvasGradientStops(deck: DeckId): ReadonlyArray<[number, string]> {
  const { center, mid, edge } = DECK_WAVEFORM_PALETTE[deck].gradient;
  return [[0, edge], [0.25, mid], [0.5, center], [0.75, mid], [1, edge]];
}

/** Amplitude thresholds for the LEVEL mode: loud, medium, quiet. */
export const LEVEL_COLORS = { loud: '#ff4d4d', medium: '#ffae33', quiet: '#4589ff' } as const;

export const WAVEFORM_CHROME = {
  background: '#0b0f15',
  overviewBackground: '#0e1219',
  placeholderText: '#5d6878',
  placeholderGrid: '#161c25',
  playhead: '#ff2a2f',
  playheadGlow: 'rgba(255, 42, 47, 0.25)',
  cue: '#f0a000',
  beat: 'rgba(255, 255, 255, 0.18)',
  downbeat: 'rgba(255, 255, 255, 0.45)',
  loopActiveFill: 'rgba(0, 200, 104, 0.16)',
  loopActiveEdge: 'rgba(0, 216, 115, 0.9)',
  loopInactiveFill: 'rgba(148, 163, 184, 0.08)',
  loopInactiveEdge: 'rgba(148, 163, 184, 0.45)',
  overviewLoopActive: 'rgba(0, 200, 104, 0.42)',
  overviewLoopInactive: 'rgba(148, 163, 184, 0.2)',
  overviewWindow: 'rgba(255, 255, 255, 0.10)',
  overviewPlayhead: '#f3f6fb',
} as const;

/** Convert `#rrggbb` to normalized RGB floats for WebGL uniforms. */
export function hexToRgbFloat(hex: string): [number, number, number] {
  const value = hex.replace('#', '').slice(0, 6);
  const n = Number.parseInt(value, 16);
  return [((n >> 16) & 255) / 255, ((n >> 8) & 255) / 255, (n & 255) / 255];
}
