import { loadLocalThreeBand, type LocalThreeBand } from './localThreeBand';
import { loadProviderThreeBand, type ProviderThreeBand } from './providerThreeBand';

export interface TrackThreeBand {
  source: 'spotify' | 'local';
  sourceFingerprint: string;
  durationSeconds: number;
  low: number[];
  mid: number[];
  high: number[];
  stale?: boolean;
}

export interface TrackThreeBandLoad {
  data: TrackThreeBand | null;
  providerError?: unknown;
  localError?: unknown;
}

type PendingLoad = { promise: Promise<TrackThreeBandLoad>; owners: Set<() => boolean> };
const pendingLoads = new Map<string, PendingLoad>();

/** Resolve one source-bound three-band view, preferring Spotify before local fallback. */
export function loadTrackThreeBand(songId: string, fingerprint: string, sessionGeneration: number, isCurrent: () => boolean = () => true): Promise<TrackThreeBandLoad> {
  const key = `${songId}\0${fingerprint}\0${sessionGeneration}`;
  const pending = pendingLoads.get(key);
  if (pending) {
    pending.owners.add(isCurrent);
    return pending.promise;
  }
  const owners = new Set([isCurrent]);
  const shouldRetry = () => [...owners].some(owner => {
    try { return owner(); } catch { return false; }
  });
  const request = (async (): Promise<TrackThreeBandLoad> => {
    let providerError: unknown;
    try {
      const provider = await loadProviderThreeBand(songId);
      if (provider?.sourceFingerprint === fingerprint) return { data: fromProvider(provider) };
    } catch (error) {
      providerError = error;
    }

    try {
      const local = await loadLocalThreeBand(songId, fingerprint, shouldRetry);
      if (local?.sourceFingerprint === fingerprint) return { data: fromLocal(local), providerError };
      return { data: null, providerError };
    } catch (localError) {
      return { data: null, providerError, localError };
    }
  })();
  const entry = { promise: request, owners };
  pendingLoads.set(key, entry);
  void request.finally(() => {
    if (pendingLoads.get(key) === entry) pendingLoads.delete(key);
  }).catch(() => {});
  return request;
}

function fromProvider(provider: ProviderThreeBand): TrackThreeBand {
  // Provider samples are signed peaks; DJ magnitude rendering uses absolute
  // envelopes and scales them together without changing the retained payload.
  return {
    source: 'spotify',
    sourceFingerprint: provider.sourceFingerprint,
    durationSeconds: provider.durationSeconds,
    low: provider.lows.map(Math.abs),
    mid: provider.mids.map(Math.abs),
    high: provider.highs.map(Math.abs),
    stale: provider.stale,
  };
}

function fromLocal(local: LocalThreeBand): TrackThreeBand {
  return {
    source: 'local',
    sourceFingerprint: local.sourceFingerprint,
    durationSeconds: local.overview.frames / local.overview.sampleRate,
    low: local.overview.low,
    mid: local.overview.mid,
    high: local.overview.high,
  };
}