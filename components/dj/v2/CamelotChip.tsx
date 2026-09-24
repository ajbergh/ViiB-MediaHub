import React from 'react';
import { getCamelotColor } from '../../../lib/camelotColors';

export interface CamelotChipProps {
  camelotKey?: string | null;
  /** Visible fallback for legacy rows that only have a canonical key. */
  fallbackLabel?: string;
  title?: string;
  compatibility?: number | null;
}

function compatibilityClass(score: number | null | undefined): string | undefined {
  if (score == null) return undefined;
  if (score >= 0.85) return 'ring-1 ring-green-300';
  if (score >= 0.7) return 'ring-1 ring-yellow-300';
  if (score >= 0.5) return 'ring-1 ring-orange-300';
  return 'ring-1 ring-neutral-500';
}

/** Compact Camelot label with deterministic position color and optional compatibility cue. */
export function CamelotChip({ camelotKey, fallbackLabel, title, compatibility }: CamelotChipProps) {
  const color = getCamelotColor(camelotKey);
  const label = color ? camelotKey!.trim().toUpperCase() : fallbackLabel || camelotKey || '-';
  const compat = compatibilityClass(compatibility);

  return (
    <span
      className={`inline-flex min-w-[1.8rem] items-center justify-center rounded px-1 py-0.5 font-mono text-[10px] leading-none ${compat || ''} ${color ? '' : 'border border-neutral-700 bg-neutral-900 text-neutral-400'}`}
      style={color ? {
        backgroundColor: color.background,
        border: `1px solid ${color.border}`,
        color: color.foreground,
      } : undefined}
      title={title}
      aria-label={title || `Camelot key ${label}`}
    >
      {label}
    </span>
  );
}

export default CamelotChip;
