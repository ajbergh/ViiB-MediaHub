/** Shares session-scoped analysis features and refreshes only changed tracks. */
import { useEffect, useSyncExternalStore } from 'react';
import { api, type TrackAnalysisFeature } from '../services/api';
import { useStore } from '../store';
import type { AnalysisListLoadState } from '../lib/analysisReadiness';

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

const emptyAnalysisRows = new AnalysisRows();
const empty: AnalysisSnapshot = { generation: -1, status: 'loading', rows: emptyAnalysisRows };
let snapshot = empty;
let fullRefresh = true;
let pending = new Set<string>();
let running: AbortController | undefined;
let timer: ReturnType<typeof setTimeout> | undefined;
let listening = false;
const listeners = new Set<() => void>();

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

function invalidate(ids?: string[]) {
  if (!ids) {
    fullRefresh = true;
    pending.clear();
  } else if (!fullRefresh) {
    ids.forEach(id => pending.add(id));
    if (pending.size > 1000) {
      fullRefresh = true;
      pending.clear();
    }
  }
  schedule();
}

async function refresh() {
  if (!fullRefresh && pending.size === 0) return;
  const generation = snapshot.generation;
  const all = fullRefresh;
  const ids = [...pending];
  fullRefresh = false;
  pending.clear();
  const controller = new AbortController();
  running = controller;
  const options = { summary: true, signal: controller.signal };
  let failed = false;
  const current = () => snapshot.generation === generation && useStore.getState().spotifySessionGeneration === generation;
  try {
    let features: TrackAnalysisFeature[] = [];
    if (all) {
      features = await api.getTrackAnalysisFeatures(undefined, options);
    } else {
      for (let start = 0; start < ids.length; start += 100) {
        features.push(...await api.getTrackAnalysisFeatures(ids.slice(start, start + 100), options));
        if (!current()) return;
      }
    }
    if (!current() || fullRefresh) return;
    const rows = (all ? emptyAnalysisRows : snapshot.rows).update(ids, features, pending, all);
    publish({ generation, rows, status: 'loaded' });
  } catch {
    if (current()) {
      failed = true;
      if (all) fullRefresh = true;
      else ids.forEach(id => pending.add(id));
      publish({ ...snapshot, status: "failed" });
    }
  } finally {
    if (running === controller) {
      running = undefined;
      if (!failed && (fullRefresh || pending.size)) schedule();
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
  invalidate();
}

export function useAnalysisFeatures(enabled = true) {
  const generation = useStore(state => state.spotifySessionGeneration);
  const state = useSyncExternalStore(enabled ? subscribe : skipSubscription, readSnapshot, () => empty);
  useEffect(() => {
    if (!enabled) return;
    if (!listening) {
      window.addEventListener('library_updated', onLibraryUpdate);
      listening = true;
    }
    if (snapshot.generation !== generation) {
      running?.abort();
      running = undefined;
      publish({ ...empty, generation });
      invalidate();
    } else if (snapshot.status === 'failed') {
      retry();
    } else if (fullRefresh || pending.size) {
      schedule();
    }
  }, [enabled, generation]);
  return { ...(enabled && state.generation === generation ? state : empty), retry };
}
