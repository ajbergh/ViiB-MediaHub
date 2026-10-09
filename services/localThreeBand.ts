import { withTransientReadRetry } from './audioReadDiagnostics';

export interface LocalThreeBand {
  songId: string;
  sourceFingerprint: string;
  representation: 'local_three_band_estimate';
  algorithmVersion: string;
  overview: {
    sampleRate: number;
    frames: number;
    resolution: number;
    low: number[];
    mid: number[];
    high: number[];
    units: string;
    filter: string;
    normalization: string;
  };
}

type PendingLoad = { promise: Promise<LocalThreeBand | null>; owners: Set<() => boolean> };
const pendingLoads = new Map<string, PendingLoad>();

/** Coalesce simultaneous overview and scrolling-lane reads for the same song revision. */
export function loadLocalThreeBand(songId: string, sourceFingerprint?: string, isCurrent: () => boolean = () => true): Promise<LocalThreeBand | null> {
  const key = `${songId}\0${sourceFingerprint ?? ''}`;
  const pending = pendingLoads.get(key);
  if (pending) {
    pending.owners.add(isCurrent);
    return pending.promise;
  }

  const owners = new Set([isCurrent]);
  const shouldRetry = () => [...owners].some(owner => {
    try { return owner(); } catch { return false; }
  });
  const request = fetchLocalThreeBand(songId, sourceFingerprint, shouldRetry);
  const entry = { promise: request, owners };
  pendingLoads.set(key, entry);
  void request.finally(() => {
    if (pendingLoads.get(key) === entry) pendingLoads.delete(key);
  }).catch(() => {});
  return request;
}

async function fetchLocalThreeBand(songId: string, sourceFingerprint: string | undefined, shouldRetry: () => boolean): Promise<LocalThreeBand | null> {
  const response = await withTransientReadRetry(async () => {
    const value = await fetch(`/api/v2/analysis/${encodeURIComponent(songId)}/waveform/local-three-band`, { cache: 'no-store' });
    if (value.ok || value.status === 404 || value.status === 412) return value;
    throw Object.assign(new Error('Local waveform unavailable'), { status: value.status });
  }, shouldRetry);
  // A source revision changed during the read; never render the old artifact.
  if (response.status === 404 || response.status === 412) return null;
  const data = await response.json() as LocalThreeBand;
  const overview = data.overview;
  if (data.songId !== songId || data.representation !== 'local_three_band_estimate'
    || data.algorithmVersion !== 'onepole-band-peaks-v1' || !data.sourceFingerprint
    || (sourceFingerprint !== undefined && data.sourceFingerprint !== sourceFingerprint)
    || !overview || overview.units !== 'filtered_pcm_absolute_peak'
    || overview.filter !== 'one_pole_250_4000_hz_residual_v1'
    || overview.normalization !== 'none_equal_channel_mono'
    || !Number.isFinite(overview.frames) || overview.frames <= 0
    || !Number.isFinite(overview.sampleRate) || overview.sampleRate <= 0
    || !Number.isInteger(overview.resolution) || overview.resolution <= 0
    || !Array.isArray(overview.low) || !Array.isArray(overview.mid) || !Array.isArray(overview.high)
    || overview.low.length === 0 || overview.low.length > 100000
    || overview.mid.length !== overview.low.length || overview.high.length !== overview.low.length
    || Math.ceil(overview.frames / overview.resolution) !== overview.low.length) {
    throw Object.assign(new Error('Invalid local waveform'), { category: 'invalid_payload' });
  }
  for (const band of [overview.low, overview.mid, overview.high]) {
    for (const value of band) if (!Number.isFinite(value) || value < 0) throw Object.assign(new Error('Invalid local waveform'), { category: 'invalid_payload' });
  }
  return data;
}

// Display-only max pooling: retain transients without altering stored samples.
export function bandDisplay(bands: number[][], width = 600): { paths: string[]; peak: number } {
  let peak = 0;
  for (const band of bands) for (const value of band) peak = Math.max(peak, value);
  const paths = bands.map(band => {
    const count = Math.min(width, band.length);
    const points: string[] = [];
    for (let i = 0; i < count; i++) {
      let value = 0;
      for (let j = Math.floor(i * band.length / count); j < Math.floor((i + 1) * band.length / count); j++) value = Math.max(value, band[j]);
      const x = count === 1 ? 0 : i * 600 / (count - 1);
      points.push(`${i === 0 ? 'M' : 'L'}${x.toFixed(2)},${(70 - (peak === 0 ? 0 : value / peak * 65)).toFixed(2)}`);
    }
    return points.join(' ');
  });
  return { paths, peak };
}
