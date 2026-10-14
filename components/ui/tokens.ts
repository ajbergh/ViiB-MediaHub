/** Defines shared design tokens and color/class conversion helpers. */

export const VIIB = {
  colors: {
    surface: {
      background: 'surface-0',
      dark: 'surface-1',
      raised: 'surface-2',
      divider: 'surface-3',
    },
    text: {
      primary: 'text-text-main',
      secondary: 'text-text-secondary',
      muted: 'text-text-subtle',
    },
    accent: {
      brand: 'brand',
      playback: 'accent-green',
      discovery: 'accent-orange',
      stats: 'accent-blue',
      destructive: 'accent-crimson',
    },
  },
  radius: {
    card: 'rounded-lg',
    control: 'rounded-md',
    pill: 'rounded-full',
  },
  motion: {
    hoverMs: 150,
    transitionMs: 200,
    easing: 'ease-out',
  },
  layout: {
    sectionPadding: 'px-6 py-5',
    maxContent: 'max-w-[1600px]',
  },
} as const;

// Use only when a component requires a literal color string (e.g., canvas/WebAudio visualizers).
export const VIIB_COLOR_VALUES = {
  brandPurple: '#1ED760',
  playbackGreen: '#1ED760',
  discoveryOrange: '#FFA42B',
  statsBlue: '#3D91F4',
  destructiveCrimson: '#E91429',
  visualizerMuted: 'rgba(255,255,255,0.4)',
} as const;

// Use only for canvas/WebAudio where dynamic alpha is required.
export const VIIB_COLOR_RGB = {
  visualizerPurple: { r: 30, g: 215, b: 96 },
  visualizerPink: { r: 59, g: 228, b: 119 },
  visualizerBlue: { r: 61, g: 145, b: 244 },
  visualizerGreen: { r: 30, g: 215, b: 96 },
  spotifyGreen: { r: 30, g: 215, b: 96 },
  white: { r: 255, g: 255, b: 255 },
  black: { r: 0, g: 0, b: 0 },
} as const;

export function rgbaFromRgb(rgb: { r: number; g: number; b: number }, alpha: number): string {
  return `rgba(${rgb.r}, ${rgb.g}, ${rgb.b}, ${alpha})`;
}

export type ViibAccent = 'brand' | 'playback' | 'discovery' | 'stats' | 'destructive';

export function accentToRingClass(accent: ViibAccent): string {
  switch (accent) {
    case 'brand':
      return 'ring-brand/40';
    case 'playback':
      return 'ring-accent-green/40';
    case 'discovery':
      return 'ring-accent-orange/40';
    case 'stats':
      return 'ring-accent-blue/40';
    case 'destructive':
      return 'ring-accent-crimson/50';
  }
}

export function accentToBgClass(accent: ViibAccent): string {
  switch (accent) {
    case 'brand':
      return 'bg-brand';
    case 'playback':
      return 'bg-accent-green';
    case 'discovery':
      return 'bg-accent-orange';
    case 'stats':
      return 'bg-accent-blue';
    case 'destructive':
      return 'bg-accent-crimson';
  }
}
