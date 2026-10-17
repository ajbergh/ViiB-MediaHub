import { expect, it } from 'vitest';
import { currentDJThreeBands, type DJBandSnapshot } from './useDJThreeBands';
import type { Song } from '../types';

it('rejects replaced-track, session and duration snapshots even before React re-renders', () => {
  const track = { id: 'one' } as Song;
  const snapshot: DJBandSnapshot = { track, session: 2, key: 'source', bands: { peak: 1, data: {
    source: 'local', sourceFingerprint: 'fp', durationSeconds: 10, low: [1], mid: [0.5], high: [0.25],
  } } };
  expect(currentDJThreeBands(snapshot, track, 10, 2)).toBe(snapshot.bands);
  expect(currentDJThreeBands(snapshot, { ...track }, 10, 2)).toBeUndefined();
  expect(currentDJThreeBands(snapshot, track, 10, 3)).toBeUndefined();
  expect(currentDJThreeBands(snapshot, track, 11, 2)).toBeUndefined();
  expect(currentDJThreeBands(undefined, null, 0, 2)).toBeUndefined();
});
