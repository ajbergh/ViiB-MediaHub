import { describe, expect, it } from 'vitest';
import { albumIdentity, albumRoute } from './albumIdentity';

describe('album identity', () => {
  it('keeps same-named albums from different artists distinct', () => {
    expect(albumIdentity('Greatest Hits', 'Artist A')).not.toBe(albumIdentity('Greatest Hits', 'Artist B'));
  });
  it('encodes route segments', () => {
    expect(albumRoute('A/B', 'AC & DC')).toBe('/album/A%2FB/AC%20%26%20DC');
  });
});
