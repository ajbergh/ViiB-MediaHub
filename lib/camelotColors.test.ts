import { describe, expect, it } from 'vitest';
import { CAMELOT_BASE_HUES, getCamelotColor } from './camelotColors';

describe('Camelot color palette', () => {
  it('assigns the specified ViiB hue to every one of the 24 Camelot keys', () => {
    for (const [number, hue] of Object.entries(CAMELOT_BASE_HUES)) {
      expect(getCamelotColor(`${number}A`)?.hue).toBe(hue);
      expect(getCamelotColor(`${number}B`)?.hue).toBe(hue);
    }
  });

  it('is deterministic and differentiates A/B using tone within the same hue family', () => {
    for (let number = 1; number <= 12; number += 1) {
      const minor = getCamelotColor(`${number}A`);
      const major = getCamelotColor(`${number}B`);
      expect(getCamelotColor(`${number}A`)).toEqual(minor);
      expect(minor?.hue).toBe(major?.hue);
      expect(minor?.background).not.toBe(major?.background);
      expect(minor?.border).not.toBe(major?.border);
    }
  });

  it('normalizes valid codes and returns null for invalid or missing values', () => {
    expect(getCamelotColor(' 4a ')?.hue).toBe(CAMELOT_BASE_HUES[4]);
    for (const invalid of ['', '0A', '13B', '4C', 'A4', null, undefined]) {
      expect(getCamelotColor(invalid)).toBeNull();
    }
  });
});
