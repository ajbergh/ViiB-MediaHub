import { describe, expect, it } from 'vitest';
import { describeKey } from './keyDetection';

describe('describeKey', () => {
  it.each([
    ['Am', 'A minor', '8A'],
    ['C#m', 'C# minor', '12A'],
    ['Dbm', 'Db minor', '12A'],
    ['F# major', 'F# major', '2B'],
    ['Gb', 'Gb major', '2B'],
    ['B minor', 'B minor', '10A'],
    ['Bbm', 'Bb minor', '3A'],
    ['c', 'C major', '8B'],
  ])('maps %s to %s / %s', (raw, label, camelot) => {
    expect(describeKey(raw)).toEqual({ label, camelot });
  });

  it('passes Camelot codes through', () => {
    expect(describeKey('8a')).toEqual({ label: '8A', camelot: '8A' });
  });

  it('keeps unrecognized text as the label', () => {
    expect(describeKey('Atonal')).toEqual({ label: 'Atonal', camelot: null });
  });

  it('returns null for empty input', () => {
    expect(describeKey(null)).toBeNull();
    expect(describeKey('  ')).toBeNull();
  });
});
