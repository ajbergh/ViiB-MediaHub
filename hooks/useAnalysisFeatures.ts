/** Shares session-scoped analysis features and refreshes only changed tracks. */
import { useEffect, useMemo, useSyncExternalStore } from 'react';
import { api, type TrackAnalysisFeature } from '../services/api';
import { useStore } from '../store';
import type { AnalysisListLoadState } from '../lib/analysisReadiness';

export interface UseAnalysisFeaturesOptions {
  enabled?: boolean;
  scalarsOnly?: boolean;
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

  update(ids: string[], features: TrackAnalysisFeature[], invalidated: ReadonlySet<string>, full: boolean) {
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
    if (full) {
      for (const field of orderingFields) revisions[field] = revision;
    } else {
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
    }
    return new AnalysisRows(buckets, revisions);
  }
}

type DetailMode = 'scalars' | 'full';

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
const pendingIds = new Map<string, DetailMode>();
const inFlightIds = new Map<string, DetailMode>();
let inFlightInvalidated = new Set<string>();
let inFlightFullInvalidated = false;
const subscribers = new Map<number, SubscriberRequest>();
let nextSubscriberId = 0;
let running: AbortController | undefined;
let timer: ReturnType<typeof setTimeout> | undefined;
let listening = false;
const listeners = new Set<() => void>();

function coversMode(existing: 'none' | DetailMode | undefined, needed: DetailMode): boolean {
  return existing === 'full' || (existing === 'scalars' && needed === 'scalars');
}

function maxMode(a: 'none' | DetailMode, b: DetailMode): DetailMode;
function maxMode(a: DetailMode | undefined, b: DetailMode): DetailMode;
function maxMode(a: 'none' | DetailMode | undefined, b: DetailMode): DetailMode {
  return a === 'full' || b === 'full' ? 'full' : 'scalars';
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
        if (!coversMode(loadedFullMode, sub.mode) && !coversMode(loadedIds.get(id), sub.mode)) {
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

function invalidate(ids?: string[]) {
  if (!ids) {
    loadedFullMode = 'none';
    loadedIds.clear();
    pendingIds.clear();
    if (running) inFlightFullInvalidated = true;
    if (subscribers.size === 0 || hasFullSubscriber()) {
      fullRefreshMode = 'full';
      for (const sub of subscribers.values()) {
        if (sub.fullLoad && sub.paged) fullRefreshPaged = true;
      }
    }
    enqueueActiveSubscribers();
  } else {
    for (const id of ids) {
      loadedIds.delete(id);
      if (running) inFlightInvalidated.add(id);
      let required: DetailMode | undefined;
      if (loadedFullMode !== 'none') {
        required = loadedFullMode;
      }
      for (const sub of subscribers.values()) {
        if (sub.fullLoad || sub.songIdSet.has(id)) {
          required = maxMode(required, sub.mode);
        }
      }
      if (required && fullRefreshMode === 'none') {
        enqueuePendingId(id, required);
      }
    }
    if (pendingIds.size > 1000 && (loadedFullMode !== 'none' || hasFullSubscriber())) {
      fullRefreshMode = 'full';
      for (const sub of subscribers.values()) {
        if (sub.fullLoad && sub.paged) fullRefreshPaged = true;
      }
      pendingIds.clear();
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
  const batchEntries = all ? [] : [...pendingIds.entries()];
  fullRefreshMode = 'none';
  fullRefreshPaged = false;
  pendingIds.clear();
  inFlightIds.clear();
  inFlightInvalidated = new Set<string>();
  inFlightFullInvalidated = false;

  const fullBatchIds: string[] = [];
  const scalarBatchIds: string[] = [];
  for (const [id, mode] of batchEntries) {
    inFlightIds.set(id, mode);
    if (mode === 'full') fullBatchIds.push(id);
    else scalarBatchIds.push(id);
  }
  const controller = new AbortController();
  running = controller;
  let failed = false;
  const current = () => running === controller && snapshot.generation === generation && useStore.getState().spotifySessionGeneration === generation;

  try {
    if (all) {
      const options = fullMode === 'scalars'
        ? { summary: true, scalarsOnly: true, signal: controller.signal }
        : { summary: true, signal: controller.signal };

      if (usePaged && typeof api.getTrackAnalysisFeaturesPage === 'function') {
        let cursor: string | undefined;
        let hasMore = true;
        const seenIds = new Set<string>();
        while (hasMore) {
          const page = await api.getTrackAnalysisFeaturesPage(cursor, 500, options);
          if (!current() || inFlightFullInvalidated || fullRefreshMode !== 'none') return;
          const invalidated = new Set<string>([...inFlightInvalidated, ...pendingIds.keys()]);
          for (const feature of page.items) {
            seenIds.add(feature.songId);
            if (!invalidated.has(feature.songId)) {
              loadedIds.set(feature.songId, maxMode(loadedIds.get(feature.songId), fullMode));
            }
          }
          hasMore = Boolean(page.hasMore && page.nextCursor && page.nextCursor !== cursor);
          cursor = page.nextCursor;
          const removedIds: string[] = [];
          if (!hasMore) {
            for (const row of snapshot.rows.values()) {
              if (!seenIds.has(row.songId) && !invalidated.has(row.songId)) {
                removedIds.push(row.songId);
                loadedIds.delete(row.songId);
              }
            }
            loadedFullMode = maxMode(loadedFullMode, fullMode);
          }
          const nextRows = snapshot.rows.update(removedIds, page.items, invalidated, false);
          publish({ generation, rows: nextRows, status: hasMore ? 'loading' : 'loaded' });
        }
      } else {
        const features = await api.getTrackAnalysisFeatures(undefined, options);
        if (!current() || inFlightFullInvalidated || fullRefreshMode !== 'none') return;
        const invalidated = new Set<string>([...inFlightInvalidated, ...pendingIds.keys()]);
        const nextRows = emptyAnalysisRows.update([], features, invalidated, true);
        loadedFullMode = maxMode(loadedFullMode, fullMode);
        for (const feature of features) {
          if (!invalidated.has(feature.songId)) {
            loadedIds.set(feature.songId, maxMode(loadedIds.get(feature.songId), fullMode));
          }
        }
        publish({ generation, rows: nextRows, status: 'loaded' });
      }
    } else {
      const features: TrackAnalysisFeature[] = [];
      const fullOptions = { summary: true, signal: controller.signal };
      const scalarOptions = { summary: true, scalarsOnly: true, signal: controller.signal };

      for (let start = 0; start < fullBatchIds.length; start += 100) {
        const chunk = fullBatchIds.slice(start, start + 100);
        features.push(...await api.getTrackAnalysisFeatures(chunk, fullOptions));
        if (!current() || inFlightFullInvalidated || fullRefreshMode !== 'none') return;
      }
      for (let start = 0; start < scalarBatchIds.length; start += 100) {
        const chunk = scalarBatchIds.slice(start, start + 100);
        features.push(...await api.getTrackAnalysisFeatures(chunk, scalarOptions));
        if (!current() || inFlightFullInvalidated || fullRefreshMode !== 'none') return;
      }

      const invalidated = new Set<string>([...inFlightInvalidated, ...pendingIds.keys()]);
      for (const [id, mode] of batchEntries) {
        if (!invalidated.has(id)) loadedIds.set(id, maxMode(loadedIds.get(id), mode));
      }
      const nextRows = snapshot.rows.update(batchEntries.map(([id]) => id), features, invalidated, false);
      publish({ generation, rows: nextRows, status: 'loaded' });
    }
  } catch {
    if (current()) {
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
      if (!failed && (fullRefreshMode !== 'none' || pendingIds.size > 0)) schedule();
    }
  }
}

function onLibraryUpdate(event: Event) {
  const detail = (event as CustomEvent<{ source?: string; songId?: string; songIds?: string[] }>).detail;
  if (detail?.source === 'full_refresh') invalidate();
  if (detail?.songIds?.length) invalidate(detail.songIds);
  else if (detail?.songId) invalidate([detail.songId]);
}

function retry() {
  publish({ ...snapshot, status: 'loading' });
  if (subscribers.size === 0) {
    fullRefreshMode = 'full';
  } else {
    enqueueActiveSubscribers();
    if (fullRefreshMode === 'none' && pendingIds.size === 0 && hasFullSubscriber()) {
      fullRefreshMode = 'full';
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
  const mode: DetailMode = scalarsOnly ? 'scalars' : 'full';
  const songIdsKey = useMemo(() => (songIds ? songIds.join('\u0000') : ''), [songIds]);
  const subId = useMemo(() => ++nextSubscriberId, []);

  const generation = useStore(state => state.spotifySessionGeneration);
  const state = useSyncExternalStore(enabled ? subscribe : skipSubscription, readSnapshot, () => empty);

  useEffect(() => () => {
    subscribers.delete(subId);
  }, [subId]);

  useEffect(() => {
    if (!enabled) {
      subscribers.delete(subId);
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

    if (snapshot.generation !== generation) {
      running?.abort();
      running = undefined;
      clearTimeout(timer);
      timer = undefined;
      loadedFullMode = 'none';
      loadedIds.clear();
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
        if (coversMode(loadedFullMode, mode) || coversMode(loadedIds.get(id), mode)) continue;
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
