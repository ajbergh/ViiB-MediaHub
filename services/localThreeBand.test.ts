import { afterEach, expect, it, vi } from 'vitest';
import { bandDisplay, loadLocalThreeBand } from './localThreeBand';
afterEach(() => vi.unstubAllGlobals());
it('max pools transients with one shared scale and handles silence', () => {
  const display = bandDisplay([[0, 1, 0, 0], [0, .5, 0, 0], [0, 0, 0, 0]], 2);
  expect(display.peak).toBe(1);
  expect(display.paths[0]).toContain('5.00');
  expect(display.paths[1]).toContain('37.50');
  expect(bandDisplay([[0], [0], [0]]).paths.join('')).not.toContain('NaN');
});
it('treats missing preparation as absent and rejects malformed bands', async () => {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ status: 404 }));
  expect(await loadLocalThreeBand('song')).toBeNull();
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ status: 200, ok: true, json: async () => ({ songId: 'song', representation: 'local_three_band_estimate', overview: { frames: 1, sampleRate: 48000, resolution: 1, low: [1], mid: [], high: [0] } }) }));
  await expect(loadLocalThreeBand('song')).rejects.toThrow('Invalid local waveform');
});

it('coalesces simultaneous overview and scrolling-lane requests by source fingerprint', async () => {
  let resolve!: (value: unknown) => void;
  const fetchMock = vi.fn(() => new Promise<{ status: number; ok: boolean; json?: () => Promise<unknown> }>(resolveRequest => { resolve = resolveRequest; }));
  vi.stubGlobal('fetch', fetchMock);
  const overview = loadLocalThreeBand('song', 'source-v1');
  const scrolling = loadLocalThreeBand('song', 'source-v1');
  expect(fetchMock).toHaveBeenCalledTimes(1);
  resolve({
    status: 200, ok: true,
    json: async () => ({
      songId: 'song', sourceFingerprint: 'source-v1', representation: 'local_three_band_estimate',
      algorithmVersion: 'onepole-band-peaks-v1',
      overview: { sampleRate: 10000, frames: 20, resolution: 10, low: [1, 0], mid: [0.5, 0], high: [0.25, 0], units: 'filtered_pcm_absolute_peak', filter: 'one_pole_250_4000_hz_residual_v1', normalization: 'none_equal_channel_mono' },
    }),
  });
  const [first, second] = await Promise.all([overview, scrolling]);
  expect(first).toBe(second);
  expect(first?.sourceFingerprint).toBe('source-v1');

  let resolveNext!: (value: unknown) => void;
  fetchMock.mockImplementationOnce(() => new Promise(resolveRequest => { resolveNext = resolveRequest; }));
  const changedSource = loadLocalThreeBand('song', 'source-v2');
  expect(fetchMock).toHaveBeenCalledTimes(2);
  resolveNext({
    status: 200, ok: true,
    json: async () => ({
      songId: 'song', sourceFingerprint: 'source-v1', representation: 'local_three_band_estimate',
      algorithmVersion: 'onepole-band-peaks-v1',
      overview: { sampleRate: 10000, frames: 20, resolution: 10, low: [1, 0], mid: [0.5, 0], high: [0.25, 0], units: 'filtered_pcm_absolute_peak', filter: 'one_pole_250_4000_hz_residual_v1', normalization: 'none_equal_channel_mono' },
    }),
  });
  await expect(changedSource).rejects.toThrow('Invalid local waveform');

  fetchMock.mockImplementationOnce(() => Promise.resolve({
    status: 200, ok: true,
    json: async () => ({
      songId: 'song', sourceFingerprint: 'source-v2', representation: 'local_three_band_estimate',
      algorithmVersion: 'onepole-band-peaks-v1',
      overview: { sampleRate: 10000, frames: 20, resolution: 10, low: [1, 0], mid: [0.5, 0], high: [0.25, 0], units: 'filtered_pcm_absolute_peak', filter: 'one_pole_250_4000_hz_residual_v1', normalization: 'none_equal_channel_mono' },
    }),
  }));
  const retriedSource = loadLocalThreeBand('song', 'source-v2');
  expect(fetchMock).toHaveBeenCalledTimes(3);
  await expect(retriedSource).resolves.toMatchObject({ sourceFingerprint: 'source-v2' });
});

it('treats a source-change race as unavailable and allows a subsequent read', async () => {
 const fetchMock = vi.fn().mockResolvedValueOnce({status:412,ok:false}).mockResolvedValueOnce({status:404,ok:false});
 vi.stubGlobal('fetch',fetchMock);
 await expect(loadLocalThreeBand('source-race','fp')).resolves.toBeNull();
 await expect(loadLocalThreeBand('source-race','fp')).resolves.toBeNull();
 expect(fetchMock).toHaveBeenCalledTimes(2);
});
