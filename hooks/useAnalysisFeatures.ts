/** Shares session-scoped analysis features and refreshes only changed tracks. */
import { useEffect, useSyncExternalStore } from 'react';
import { api, type TrackAnalysisFeature } from '../services/api';
import { useStore } from '../store';
import type { AnalysisListLoadState } from '../lib/analysisReadiness';

interface AnalysisSnapshot {
  generation: number;
  status: AnalysisListLoadState;
  rows: Record<string, TrackAnalysisFeature>;
}

const empty: AnalysisSnapshot = { generation: -1, status: 'loading', rows: {} };
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
    const rows = all ? {} : { ...snapshot.rows };
    ids.forEach(id => { delete rows[id]; });
    features.forEach(feature => {
      if (!pending.has(feature.songId)) rows[feature.songId] = feature;
    });
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
