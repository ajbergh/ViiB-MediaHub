import { describe, expect, it } from 'vitest';
import { compareIntegratedLUFS, formatIntegratedLUFS, isFiniteIntegratedLUFS } from './djLibraryLUFS';

describe('DJ library integrated LUFS', () => {
  it('formats only finite measurements and leaves missing values absent', () => {
    expect(formatIntegratedLUFS(-14.26)).toBe('-14.3');
    expect(formatIntegratedLUFS(undefined)).toBe('—');
    expect(formatIntegratedLUFS(Number.NaN)).toBe('—');
    expect(formatIntegratedLUFS(Number.POSITIVE_INFINITY)).toBe('—');
    expect(isFiniteIntegratedLUFS(-14.26)).toBe(true);
    expect(isFiniteIntegratedLUFS(undefined)).toBe(false);
    expect(isFiniteIntegratedLUFS(Number.NaN)).toBe(false);
  });

  it('sorts known values both ways and keeps unknowns last', () => {
    const values = [-12, undefined, -18, Number.NaN];
    const ascending = [...values].sort((a, b) => compareIntegratedLUFS(a, b, 'asc'));
    const descending = [...values].sort((a, b) => compareIntegratedLUFS(a, b, 'desc'));
    expect(ascending.slice(0, 2)).toEqual([-18, -12]);
    expect(descending.slice(0, 2)).toEqual([-12, -18]);
    expect(ascending.slice(2).every(value => !isFiniteIntegratedLUFS(value))).toBe(true);
    expect(descending.slice(2).every(value => !isFiniteIntegratedLUFS(value))).toBe(true);
  });
});
