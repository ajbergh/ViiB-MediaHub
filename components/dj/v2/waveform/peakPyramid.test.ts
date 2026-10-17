import { expect, it } from 'vitest';
import { PeakPyramid } from './peakPyramid';

it('matches exact range maxima at every boundary, including a final unpaired transient', () => {
  const source = Array.from({ length: 137 }, (_, i) => Math.abs(Math.sin(i * 3.7)));
  source[136] = 1.5;
  const pyramid = new PeakPyramid(source);
  for (let first = 0; first < source.length; first++) for (let end = first + 1; end <= source.length; end++) {
    expect(pyramid.max(first, end)).toBe(Math.max(0, ...source.slice(first, end)));
  }
  expect(pyramid.max(-10, 999)).toBe(1.5);
  expect(pyramid.max(12, 12)).toBe(0);
});
