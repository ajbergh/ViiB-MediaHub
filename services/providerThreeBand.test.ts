import { expect, it } from 'vitest';
import { signedBandPaths } from './providerThreeBand';
it('preserves sign on a common scale and handles silence', () => {
  const paths = signedBandPaths([[-10, 10], [-5, 5], [0, 0]]);
  expect(paths[0]).toContain('75.00'); expect(paths[0]).toContain('5.00');
  expect(paths[1]).toContain('57.50'); expect(paths[1]).toContain('22.50');
  expect(paths[2]).not.toContain('NaN');
});
