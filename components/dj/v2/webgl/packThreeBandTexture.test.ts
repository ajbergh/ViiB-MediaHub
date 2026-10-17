import { expect, it } from 'vitest';
import { packThreeBandTexture } from './packThreeBandTexture';

it('packs low/mid/high into RGB with a common scale and max-pools transients under texture limits', () => {
  const packed = packThreeBandTexture({ peak: 2, data: { source: 'local', sourceFingerprint: 'fp', durationSeconds: 10,
    low: [0, 2, 0, 0, 0, 0, 0, 1], mid: Array(8).fill(1), high: Array(8).fill(0.5) } }, 2);
  expect([packed.width, packed.height, packed.count]).toEqual([2, 2, 4]);
  expect([...packed.pixels.slice(0, 4)]).toEqual([255, 128, 64, 255]);
  expect([...packed.pixels.slice(12, 16)]).toEqual([128, 128, 64, 255]);
});

it('represents silent three-band data without dividing by zero', () => {
  const packed = packThreeBandTexture({ peak: 0, data: { source: 'local', sourceFingerprint: 'fp', durationSeconds: 1,
    low: [0], mid: [0], high: [0] } }, 8192);
  expect([...packed.pixels]).toEqual([0, 0, 0, 255]);
});
