export type TruePeakOrder = 'asc' | 'desc';

export function isFiniteTruePeakDBTP(value?: number): value is number {
  return value !== undefined && Number.isFinite(value);
}

export function formatTruePeakDBTP(value?: number): string {
  return isFiniteTruePeakDBTP(value) ? value.toFixed(1) : '—';
}

/** Sort true-peak measurements in the selected direction while keeping unknowns last. */
export function compareTruePeakDBTP(a?: number, b?: number, order: TruePeakOrder = 'asc'): number {
  const aKnown = isFiniteTruePeakDBTP(a);
  const bKnown = isFiniteTruePeakDBTP(b);
  if (aKnown !== bKnown) return aKnown ? -1 : 1;
  if (!aKnown || !bKnown) return 0;
  const difference = a - b;
  return order === 'asc' ? difference : -difference;
}
