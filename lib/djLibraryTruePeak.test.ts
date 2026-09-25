import { describe, expect, it } from 'vitest';
import { compareTruePeakDBTP, formatTruePeakDBTP, isFiniteTruePeakDBTP } from './djLibraryTruePeak';

describe('DJ library True Peak', () => {
  it('formats only finite measurements and leaves missing values absent', () => {
    expect(formatTruePeakDBTP(-0.42)).toBe('-0.4');
    expect(formatTruePeakDBTP(undefined)).toBe('—');
    expect(formatTruePeakDBTP(Number.NaN)).toBe('—');
    expect(formatTruePeakDBTP(Number.NEGATIVE_INFINITY)).toBe('—');
    expect(isFiniteTruePeakDBTP(-0.42)).toBe(true);
    expect(isFiniteTruePeakDBTP(undefined)).toBe(false);
    expect(isFiniteTruePeakDBTP(Number.NaN)).toBe(false);
  });

  it('sorts known values both ways and keeps unknowns last', () => {
    const values = [-0.2, undefined, -1.8, Number.NaN];
    const ascending = [...values].sort((a, b) => compareTruePeakDBTP(a, b, 'asc'));
    const descending = [...values].sort((a, b) => compareTruePeakDBTP(a, b, 'desc'));
    expect(ascending.slice(0, 2)).toEqual([-1.8, -0.2]);
    expect(descending.slice(0, 2)).toEqual([-0.2, -1.8]);
    expect(ascending.slice(2).every(value => !isFiniteTruePeakDBTP(value))).toBe(true);
    expect(descending.slice(2).every(value => !isFiniteTruePeakDBTP(value))).toBe(true);
  });
});
