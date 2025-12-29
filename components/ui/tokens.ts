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
    card: 'rounded-xl',
    control: 'rounded-lg',
    pill: 'rounded-full',
  },
  motion: {
    hoverMs: 150,
    transitionMs: 200,
    easing: 'ease-out',
  },
  layout: {
    sectionPadding: 'p-6',
    maxContent: 'max-w-[1440px]',
  },
} as const;

// Use only when a component requires a literal color string (e.g., canvas/WebAudio visualizers).
export const VIIB_COLOR_VALUES = {
  brandPurple: '#9B5CFF',
  playbackGreen: '#3EE089',
  discoveryOrange: '#FF9F43',
  statsBlue: '#4EA1FF',
  destructiveCrimson: '#FF5D5D',
  visualizerMuted: 'rgba(255,255,255,0.4)',
} as const;

// Use only for canvas/WebAudio where dynamic alpha is required.
export const VIIB_COLOR_RGB = {
  visualizerPurple: { r: 139, g: 92, b: 246 },
  visualizerPink: { r: 236, g: 72, b: 153 },
  visualizerBlue: { r: 59, g: 130, b: 246 },
  visualizerGreen: { r: 34, g: 197, b: 94 },
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
