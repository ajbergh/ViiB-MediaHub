/** Shares session-scoped analysis features and refreshes only changed tracks. */
import { useEffect, useMemo, useSyncExternalStore } from 'react';
import { api, type TrackAnalysisFeature } from '../services/api';
import { useStore } from '../store';
import type { AnalysisListLoadState } from '../lib/analysisReadiness';

export interface UseAnalysisFeaturesOptions {
  enabled?: boolean;
  scalarsOnly?: boolean;
  projection?: 'ordering' | 'loudness';
  songIds?: readonly string[];
  fullLoad?: boolean;
  paged?: boolean;
}

interface AnalysisSnapshot {
  generation: number;
  status: AnalysisListLoadState;
  rows: AnalysisRows;
}

const orderingFields = [
  'bpm', 'key', 'camelotKey', 'energyLevel', 'status', 'analyzedAt',
  'integratedLufsBs1770', 'truePeakDbtp', 'providerScores', 'providerScoresUnverified',
] as const;
export type AnalysisOrderingField = typeof orderingFields[number];
let orderingRevision = 0;

// Preserve a stable snapshot for React while copying only buckets touched by a delta.
class AnalysisRows {
  constructor(
    private readonly buckets = new Map<number, ReadonlyMap<string, TrackAnalysisFeature>>(),
    private readonly revisions: Partial<Record<AnalysisOrderingField, number>> = {},
  ) {}

  private bucket(id: string) {
    let hash = id.length;
    // File and provider IDs may share a prefix. Include the suffix without
    // walking the entire ID for every lookup in a sort comparator.
    for (let i = 0; i < Math.min(id.length, 4); i++) {
      hash = Math.imul(hash, 31) + id.charCodeAt(i) + 17 * id.charCodeAt(id.length - i - 1);
    }
    return (hash ^ (hash >>> 16)) & 255;
  }

  get(id: string) {
    return this.buckets.get(this.bucket(id))?.get(id);
  }

  *values() {
    for (const bucket of this.buckets.values()) yield* bucket.values();
  }

  revisionFor(fields: readonly AnalysisOrderingField[]) {
    return fields.reduce((revision, field) => Math.max(revision, this.revisions[field] ?? 0), 0);
  }

  update(ids: string[], features: TrackAnalysisFeature[], invalidated: ReadonlySet<string>) {
    const buckets = new Map(this.buckets);
    const changed = new Map<number, Map<string, TrackAnalysisFeature>>();
    const edit = (id: string) => {
      const key = this.bucket(id);
      let bucket = changed.get(key);
      if (!bucket) {
        bucket = new Map(this.buckets.get(key));
        buckets.set(key, bucket);
        changed.set(key, bucket);
      }
      return bucket;
    };
    for (const id of ids) if (this.get(id) && !invalidated.has(id)) edit(id).delete(id);
    for (const feature of features) {
      if (!invalidated.has(feature.songId)) edit(feature.songId).set(feature.songId, feature);
    }
    if (!changed.size) return this;

    const revisions = { ...this.revisions };
    const revision = ++orderingRevision;
    const touched = new Set([...ids, ...features.map(feature => feature.songId)]);
    for (const id of touched) {
      const bucket = changed.get(this.bucket(id));
      if (!bucket) continue;
      const before = this.get(id);
      const after = bucket.get(id);
      for (const field of orderingFields) {
        if (!Object.is(before?.[field], after?.[field]) &&
            (field !== 'providerScores' || JSON.stringify(before?.providerScores) !== JSON.stringify(after?.providerScores))) {
          revisions[field] = revision;
        }
      }
    }
    return new AnalysisRows(buckets, revisions);
  }
}

type DetailMode = 'scalars' | 'ordering' | 'loudness' | 'full';
const detailRank = { none: 0, scalars: 1, ordering: 2, loudness: 3, full: 4 } as const;

interface SubscriberRequest {
  mode: DetailMode;
  fullLoad: boolean;
  paged: boolean;
  songIds: readonly string[];
  songIdSet: ReadonlySet<string>;
}

const emptyAnalysisRows = new AnalysisRows();
const empty: AnalysisSnapshot = { generation: -1, status: 'loading', rows: emptyAnalysisRows };
let snapshot = empty;
let fullRefreshMode: 'none' | DetailMode = 'none';
let fullRefreshPaged = false;
let loadedFullMode: 'none' | DetailMode = 'none';
const loadedIds = new Map<string, DetailMode>();
const explicitCoverageIds = new Set<string>();
const pendingIds = new Map<string, DetailMode>();
const inFlightIds = new Map<string, DetailMode>();
let inFlightInvalidated = new Set<string>();
let inFlightFullInvalidated = false;
const subscribers = new Map<number, SubscriberRequest>();
let nextSubscriberId = 0;
let running: AbortController | undefined;
let catalogRunning = false;
let timer: ReturnType<typeof setTimeout> | undefined;
let listening = false;
const listeners = new Set<() => void>();

function coversMode(existing: 'none' | DetailMode | undefined, needed: DetailMode): boolean {
  return detailRank[existing ?? 'none'] >= detailRank[needed];
}

function maxMode(a: 'none' | DetailMode, b: DetailMode): DetailMode;
function maxMode(a: DetailMode | undefined, b: DetailMode): DetailMode;
function maxMode(a: 'none' | DetailMode | undefined, b: DetailMode): DetailMode {
  return a && a !== 'none' && coversMode(a, b) ? a : b;
}

function hasCoverage(id: string, mode: DetailMode) {
  return coversMode(loadedIds.get(id), mode) || (!explicitCoverageIds.has(id) && coversMode(loadedFullMode, mode));
}

function requestOptions(mode: DetailMode, signal: AbortSignal) {
  return { summary: true, signal, ...(mode === 'scalars' ? { scalarsOnly: true } :
    mode === 'full' ? {} : { projection: mode }) };
}

function enqueuePendingId(id: string, mode: DetailMode) {
  pendingIds.set(id, maxMode(pendingIds.get(id), mode));
}

function enqueueActiveSubscribers() {
  for (const sub of subscribers.values()) {
    if (sub.fullLoad) {
      if (!coversMode(loadedFullMode, sub.mode)) {
        fullRefreshMode = maxMode(fullRefreshMode, sub.mode);
        if (sub.paged) fullRefreshPaged = true;
      }
    } else {
      for (const id of sub.songIds) {
        if (!hasCoverage(id, sub.mode)) {
          enqueuePendingId(id, sub.mode);
        }
      }
    }
  }
}

function publish(next: AnalysisSnapshot) {
  snapshot = next;
  listeners.forEach(listener => listener());
}

const readSnapshot = () => snapshot;
const subscribe = (listener: () => void) => {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
    if (listeners.size === 0) {
      clearTimeout(timer);
      timer = undefined;
    }
  };
};
const skipSubscription = () => () => {};

function schedule() {
  if (running || timer !== undefined || !listeners.size) return;
  timer = setTimeout(() => {
    timer = undefined;
    if (listeners.size) void refresh();
  }, 100);
}

function hasFullSubscriber(): boolean {
  for (const sub of subscribers.values()) {
    if (sub.fullLoad) return true;
  }
  return false;
}

function requestedModeForSong(id: string): DetailMode | undefined {
  let mode: DetailMode | undefined;
  for (const sub of subscribers.values()) {
    if (sub.fullLoad || sub.songIdSet.has(id)) mode = maxMode(mode, sub.mode);
  }
  return mode;
}

function stopUnusedCatalog() {
  if (hasFullSubscriber()) return;
  fullRefreshMode = 'none';
  fullRefreshPaged = false;
  if (catalogRunning) running?.abort();
}

function invalidate(ids?: string[]) {
  if (!ids) {
    loadedFullMode = 'none';
    loadedIds.clear();
    explicitCoverageIds.clear();
    pendingIds.clear();
    if (running) inFlightFullInvalidated = true;
    fullRefreshMode = 'none';
    fullRefreshPaged = false;
    enqueueActiveSubscribers();
  } else {
    for (const id of ids) {
      loadedIds.delete(id);
      if (loadedFullMode !== 'none') explicitCoverageIds.add(id);
      if (running) inFlightInvalidated.add(id);
      const required = requestedModeForSong(id);
      if (required && !coversMode(fullRefreshMode, required)) {
        enqueuePendingId(id, required);
      }
    }
    if (pendingIds.size > 1000 && hasFullSubscriber()) {
      for (const sub of subscribers.values()) {
        if (sub.fullLoad) {
          fullRefreshMode = maxMode(fullRefreshMode, sub.mode);
          if (sub.paged) fullRefreshPaged = true;
        }
      }
    }
  }
  if (fullRefreshMode !== 'none' || pendingIds.size > 0) {
    schedule();
  }
}

async function refresh() {
  if (fullRefreshMode === 'none' && pendingIds.size === 0) return;
  const generation = snapshot.generation;
  const fullMode = fullRefreshMode;
  const all = fullMode !== 'none';
  const usePaged = fullRefreshPaged;
  const batchEntries: [string, DetailMode][] = [];
  fullRefreshMode = 'none';
  fullRefreshPaged = false;
  for (const id of pendingIds.keys()) {
    const mode = requestedModeForSong(id);
    if (all && mode && !coversMode(fullMode, mode)) continue;
    pendingIds.delete(id);
    if (!all && mode && !hasCoverage(id, mode)) batchEntries.push([id, mode]);
  }
  if (!all && !batchEntries.length) return;
  inFlightIds.clear();
  inFlightInvalidated = new Set<string>();
  inFlightFullInvalidated = false;

  const batches = new Map<DetailMode, string[]>();
  for (const [id, mode] of batchEntries) {
    inFlightIds.set(id, mode);
    const batch = batches.get(mode);
    if (batch) batch.push(id);
    else batches.set(mode, [id]);
  }
  const controller = new AbortController();
  running = controller;
  catalogRunning = all;
  let failed = false;
  const current = () => running === controller && snapshot.generation === generation && useStore.getState().spotifySessionGeneration === generation;
  const superseded = () => controller.signal.aborted || !current() || inFlightFullInvalidated || fullRefreshMode !== 'none';

  // A cheaper catalog projection can arrive after a richer viewport request.
  // Keep its artifact fields only while the existing detail is still current.
  const preserveDetails = (feature: TrackAnalysisFeature, mode: DetailMode) => {
    const cachedMode = loadedIds.get(feature.songId);
    if (!cachedMode || coversMode(mode, cachedMode)) return feature;
    const cached = snapshot.rows.get(feature.songId);
    if (!cached) return feature;
    if (cached.sourceFingerprint !== feature.sourceFingerprint) {
      loadedIds.delete(feature.songId);
      return feature;
    }
    if (mode === 'scalars') return cached;
    return {
      ...feature,
      structureAvailable: cached.structureAvailable,
      ...(mode === 'ordering' ? { integratedLufsBs1770: cached.integratedLufsBs1770, truePeakDbtp: cached.truePeakDbtp } : {}),
    };
  };

  try {
    if (all) {
      const options = requestOptions(fullMode, controller.signal);
      const features: TrackAnalysisFeature[] = [];
      if (usePaged && typeof api.getTrackAnalysisFeaturesPage === 'function') {
        if (snapshot.status !== 'loading') publish({ ...snapshot, status: 'loading' });
        let cursor: string | undefined;
        for (;;) {
          const page = await api.getTrackAnalysisFeaturesPage(cursor, 500, options);
          if (superseded()) return;
          features.push(...page.items);
          if (!page.hasMore) break;
          if (!page.nextCursor || page.nextCursor === cursor) throw new Error('Analysis cursor did not advance');
          cursor = page.nextCursor;
        }
      } else {
        for (const feature of await api.getTrackAnalysisFeatures(undefined, options)) features.push(feature);
        if (superseded()) return;
      }
      const removedIds: string[] = [];
      // Viewport requests can be queued during pagination without invalidating the catalog data.
      const invalidated = inFlightInvalidated;
      const seen = new Set(features.map(feature => feature.songId));
      for (const row of snapshot.rows.values()) {
        if (!seen.has(row.songId) && !invalidated.has(row.songId)) {
          removedIds.push(row.songId);
          loadedIds.delete(row.songId);
        }
      }
      const nextRows = snapshot.rows.update(removedIds, features.map(feature => preserveDetails(feature, fullMode)), invalidated);
      loadedFullMode = maxMode(loadedFullMode, fullMode);
      for (const id of explicitCoverageIds) {
        if (!invalidated.has(id) && coversMode(fullMode, loadedFullMode)) explicitCoverageIds.delete(id);
      }
      for (const [id, mode] of pendingIds) {
        if (hasCoverage(id, mode) && !invalidated.has(id)) pendingIds.delete(id);
      }
      publish({ generation, rows: nextRows, status: 'loaded' });
    } else {
      const features: TrackAnalysisFeature[] = [];
      for (const [mode, ids] of batches) {
        const options = requestOptions(mode, controller.signal);
        for (let start = 0; start < ids.length; start += 100) {
          const chunk = await api.getTrackAnalysisFeatures(ids.slice(start, start + 100), options);
          if (superseded()) return;
          features.push(...chunk.map(feature => preserveDetails(feature, mode)));
        }
      }

      const invalidated = new Set<string>([...inFlightInvalidated, ...pendingIds.keys()]);
      for (const [id, mode] of batchEntries) {
        if (!invalidated.has(id)) {
          loadedIds.set(id, maxMode(loadedIds.get(id), mode));
          if (coversMode(mode, loadedFullMode === 'none' ? mode : loadedFullMode)) explicitCoverageIds.delete(id);
        }
      }
      const nextRows = snapshot.rows.update(batchEntries.map(([id]) => id), features, invalidated);
      publish({ generation, rows: nextRows, status: 'loaded' });
    }
  } catch {
    if (current() && !controller.signal.aborted) {
      failed = true;
      if (all) {
        fullRefreshMode = maxMode(fullRefreshMode, fullMode);
        if (usePaged) fullRefreshPaged = true;
      } else {
        // Delta responses are published atomically, so retry every unpublished chunk.
        for (const [id, mode] of batchEntries) {
          enqueuePendingId(id, mode);
        }
      }
      publish({ ...snapshot, status: 'failed' });
    }
  } finally {
    if (running === controller) {
      inFlightIds.clear();
      inFlightInvalidated.clear();
      inFlightFullInvalidated = false;
      running = undefined;
      catalogRunning = false;
      if (!failed && (fullRefreshMode !== 'none' || pendingIds.size > 0)) schedule();
      else if (!failed && controller.signal.aborted && snapshot.status === 'loading') publish({ ...snapshot, status: 'loaded' });
    }
  }
}

function onLibraryUpdate(event: Event) {
  const detail = (event as CustomEvent<{ source?: string; songId?: string; songIds?: string[]; analysisSongIds?: string[] }>).detail;
  if (detail?.source === 'full_refresh') invalidate();
  if (detail?.analysisSongIds) invalidate(detail.analysisSongIds);
  else if (detail?.songIds?.length) invalidate(detail.songIds);
  else if (detail?.songId) invalidate([detail.songId]);
}

function retry() {
  publish({ ...snapshot, status: 'loading' });
  if (subscribers.size === 0) {
    fullRefreshMode = 'full';
  } else {
    enqueueActiveSubscribers();
    if (fullRefreshMode === 'none' && pendingIds.size === 0) {
      for (const sub of subscribers.values()) {
        if (!sub.fullLoad) continue;
        fullRefreshMode = maxMode(fullRefreshMode, sub.mode);
        fullRefreshPaged ||= sub.paged;
      }
    }
  }
  schedule();
}

export function useAnalysisFeatures(optionsOrEnabled: boolean | UseAnalysisFeaturesOptions = true) {
  const isObj = typeof optionsOrEnabled === 'object' && optionsOrEnabled !== null;
  const enabled = isObj ? (optionsOrEnabled.enabled ?? true) : Boolean(optionsOrEnabled);
  const scalarsOnly = isObj ? Boolean(optionsOrEnabled.scalarsOnly) : false;
  const songIds = isObj ? optionsOrEnabled.songIds : undefined;
  const fullLoad = isObj ? (optionsOrEnabled.fullLoad ?? songIds === undefined) : true;
  const paged = isObj ? Boolean(optionsOrEnabled.paged ?? fullLoad) : false;
  const mode: DetailMode = scalarsOnly ? 'scalars' : (isObj ? optionsOrEnabled.projection ?? 'full' : 'full');
  const songIdsKey = useMemo(() => (songIds ? songIds.join('\u0000') : ''), [songIds]);
  const subId = useMemo(() => ++nextSubscriberId, []);

  const generation = useStore(state => state.spotifySessionGeneration);
  const state = useSyncExternalStore(enabled ? subscribe : skipSubscription, readSnapshot, () => empty);

  useEffect(() => () => {
    subscribers.delete(subId);
    stopUnusedCatalog();
  }, [subId]);

  useEffect(() => {
    if (!enabled) {
      subscribers.delete(subId);
      stopUnusedCatalog();
      return;
    }
    if (!listening) {
      window.addEventListener('library_updated', onLibraryUpdate);
      listening = true;
    }

    const currentIds = songIds ?? [];
    subscribers.set(subId, {
      mode,
      fullLoad,
      paged,
      songIds: currentIds,
      songIdSet: new Set(currentIds),
    });
    if (!fullLoad) stopUnusedCatalog();

    if (snapshot.generation !== generation) {
      running?.abort();
      running = undefined;
      catalogRunning = false;
      clearTimeout(timer);
      timer = undefined;
      loadedFullMode = 'none';
      loadedIds.clear();
      explicitCoverageIds.clear();
      fullRefreshMode = 'none';
      fullRefreshPaged = false;
      pendingIds.clear();
      inFlightIds.clear();
      inFlightInvalidated.clear();
      inFlightFullInvalidated = false;
      publish({ ...empty, generation });
      enqueueActiveSubscribers();
      if (fullRefreshMode !== 'none' || pendingIds.size > 0) {
        schedule();
      } else {
        publish({ generation, rows: emptyAnalysisRows, status: 'loaded' });
      }
      return;
    }

    if (fullLoad) {
      if (!coversMode(loadedFullMode, mode)) {
        fullRefreshMode = maxMode(fullRefreshMode, mode);
        if (paged) fullRefreshPaged = true;
      }
    } else {
      for (const id of currentIds) {
        if (hasCoverage(id, mode)) continue;
        if (coversMode(inFlightIds.get(id), mode) && !inFlightInvalidated.has(id)) continue;
        enqueuePendingId(id, mode);
      }
    }

    if (snapshot.status === 'failed') {
      retry();
    } else if (fullRefreshMode !== 'none' || pendingIds.size > 0) {
      schedule();
    } else if (!running && snapshot.status === 'loading') {
      publish({ ...snapshot, status: 'loaded' });
    }
  }, [enabled, generation, mode, fullLoad, paged, songIdsKey, subId]);

  return { ...(enabled && state.generation === generation ? state : empty), retry };
}
