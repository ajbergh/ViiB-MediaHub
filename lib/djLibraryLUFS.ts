export type LUFSOrder = 'asc' | 'desc';

export function isFiniteIntegratedLUFS(value?: number): value is number {
  return value !== undefined && Number.isFinite(value);
}

export function formatIntegratedLUFS(value?: number): string {
  return isFiniteIntegratedLUFS(value) ? value.toFixed(1) : '—';
}

/** Sort numeric loudness values in the selected direction while keeping unknowns last. */
export function compareIntegratedLUFS(a?: number, b?: number, order: LUFSOrder = 'asc'): number {
  const aKnown = isFiniteIntegratedLUFS(a);
  const bKnown = isFiniteIntegratedLUFS(b);
  if (aKnown !== bKnown) return aKnown ? -1 : 1;
  if (!aKnown || !bKnown) return 0;
  const difference = a - b;
  return order === 'asc' ? difference : -difference;
}
