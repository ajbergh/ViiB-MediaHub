import { afterEach, expect, it, vi } from 'vitest';
const mocks = vi.hoisted(() => ({ provider: vi.fn(), local: vi.fn() }));
vi.mock('./providerThreeBand', async original => ({ ...await original<typeof import('./providerThreeBand')>(), loadProviderThreeBand: mocks.provider }));
vi.mock('./localThreeBand', async original => ({ ...await original<typeof import('./localThreeBand')>(), loadLocalThreeBand: mocks.local }));
afterEach(() => { vi.resetModules(); mocks.provider.mockReset(); mocks.local.mockReset(); });

const providerData = (sourceFingerprint = 'fp') => ({ songId: 'song', sourceFingerprint, recordingId: 'recording', representation: 'spotify_three_band' as const, lows: [-1, 2], mids: [-3, 4], highs: [5, -6], sampleRate: 44100, windowMilliseconds: 20, totalSamples: 2, displayStride: 1, displayAggregation: 'signed_absolute_max_v1' as const, durationSeconds: 12, localDurationSeconds: 12, alignment: 'duration_compatible' as const, retrievedAt: '2026-10-01T00:00:00Z', stale: false });
const localData = () => ({ songId: 'song', sourceFingerprint: 'fp', representation: 'local_three_band_estimate' as const, algorithmVersion: 'onepole-band-peaks-v1', overview: { sampleRate: 48000, frames: 576000, resolution: 960, low: [.1, .2], mid: [.3, .4], high: [.5, .6] } });

it('uses a valid current-source Spotify waveform without reading local bands', async () => {
 mocks.provider.mockResolvedValue(providerData());
 const { loadTrackThreeBand } = await import('./trackThreeBand');
 const result = await loadTrackThreeBand('song', 'fp', 1);
 expect(result.data).toMatchObject({ source: 'spotify', sourceFingerprint: 'fp', durationSeconds: 12, low: [1, 2], mid: [3, 4], high: [5, 6] });
 expect(mocks.local).not.toHaveBeenCalled();
});

it('uses local bands only after provider data is missing, mismatched, or unavailable', async () => {
 mocks.provider.mockResolvedValueOnce(null).mockResolvedValueOnce(providerData('old-fp')).mockRejectedValueOnce(new TypeError('temporary failure'));
 mocks.local.mockResolvedValue(localData());
 const { loadTrackThreeBand } = await import('./trackThreeBand');
 for (let attempt = 0; attempt < 3; attempt++) {
  const result = await loadTrackThreeBand('song', 'fp', 1);
  expect(result.data).toMatchObject({ source: 'local', sourceFingerprint: 'fp', durationSeconds: 12, low: [.1, .2] });
 }
 expect(mocks.local).toHaveBeenCalledTimes(3);
});

it('coalesces simultaneous provider-first reads for the same source', async () => {
 let resolve!: (value: ReturnType<typeof providerData>) => void;
 mocks.provider.mockReturnValue(new Promise(value => { resolve = value; }));
 const { loadTrackThreeBand } = await import('./trackThreeBand');
 const first = loadTrackThreeBand('song', 'fp', 1);
 const second = loadTrackThreeBand('song', 'fp', 1);
 resolve(providerData());
 const [a, b] = await Promise.all([first, second]);
 expect(a.data?.source).toBe('spotify');
 expect(b.data?.source).toBe('spotify');
 expect(mocks.provider).toHaveBeenCalledTimes(1);
 expect(mocks.local).not.toHaveBeenCalled();
});
it('does not share an in-flight waveform read across Spotify sessions', async () => {
 let resolves: Array<(value: ReturnType<typeof providerData>) => void> = [];
 mocks.provider.mockImplementation(() => new Promise(value => { resolves.push(value); }));
 const { loadTrackThreeBand } = await import('./trackThreeBand');
 const first = loadTrackThreeBand('song', 'fp', 1);
 const second = loadTrackThreeBand('song', 'fp', 2);
 expect(mocks.provider).toHaveBeenCalledTimes(2);
 resolves[0](providerData());
 resolves[1](providerData());
 const results = await Promise.all([first, second]);
 expect(results.map(result => result.data?.source)).toEqual(['spotify', 'spotify']);
 expect(mocks.local).not.toHaveBeenCalled();
});