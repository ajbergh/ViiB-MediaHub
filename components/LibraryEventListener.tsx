/** Synchronizes library changes from revisioned events with legacy-event fallback. */

import { useEffect, useRef } from 'react';
import { useStore } from '../store';
import { libraryV2 } from '../services/libraryV2';
import { getEventStreamURL } from '../services/eventStreamURL';

type LegacyLibraryEventType =
  | 'scan_started'
  | 'scan_complete'
  | 'scan_progress'
  | 'library_updated'
  | 'enrichment_started'
  | 'enrichment_progress'
  | 'enrichment_complete'
  | 'mood_started'
  | 'mood_progress'
  | 'mood_complete';

interface LegacyLibraryEvent {
  type: LegacyLibraryEventType;
  message: string;
  data?: {
    totalSongs?: number;
    processedSongs?: number;
    changedSongs?: number;
    emptyResults?: number;
    currentBatch?: number;
    totalBatches?: number;
    enrichedSongs?: number;
  };
}

interface RevisionEvent {
  revision: number;
}

/** Connects progress notifications to the shared catalog synchronization action. */
const LibraryEventListener = () => {
  const ready = useStore(state => state.backendAvailable && !state.isLibraryInitializing);
  const lastEnrichmentProgressRef = useRef("");

  useEffect(() => {
    if (!ready) return;

    let disposed = false;
    let legacySource: EventSource | null = null;
    let revisionSource: EventSource | null = null;
    let legacyReconnect: ReturnType<typeof setTimeout> | undefined;
    let revisionReconnect: ReturnType<typeof setTimeout> | undefined;
    let mutationSyncTimer: ReturnType<typeof setTimeout> | undefined;
    let revisionReconnects = 0;

    const syncNow = (recover = false) => {
      clearTimeout(mutationSyncTimer);
      mutationSyncTimer = undefined;
      return useStore.getState().syncLibraryCatalog(recover);
    };

    const syncCommittedLibraryMutation = () => {
      if (mutationSyncTimer !== undefined) return;
      mutationSyncTimer = setTimeout(() => {
        mutationSyncTimer = undefined;
        if (!disposed) void syncNow();
      }, 100);
    };

    const handleLegacyEvent = (event: MessageEvent) => {
      try {
        const payload = JSON.parse(event.data) as LegacyLibraryEvent;
        const { setScanning, setScanProgress, setEnrichmentStatus, addLog } = useStore.getState();
        switch (payload.type) {
          case 'scan_started':
            setScanning(true);
            setScanProgress(payload.message);
            break;
          case 'scan_progress':
            setScanProgress(payload.message);
            break;
          case 'scan_complete': {
            const sync = syncNow(true);
            void sync.then(() => {
              if (disposed) return;
              setScanning(false);
              setScanProgress('');
            });
            break;
          }
          case 'library_updated':
            // A local scan emits this after each committed batch. Prefer a
            // revision delta, but use the legacy full refresh if its stream is
            // not connected yet so first-run libraries populate immediately.
            if (useStore.getState().isScanning && payload.message) {
              setScanProgress(payload.message);
            }
            syncCommittedLibraryMutation();
            break;
          case 'enrichment_started':
          case 'mood_started':
			lastEnrichmentProgressRef.current = '';
            setEnrichmentStatus({
              isEnriching: true,
              totalSongs: payload.data?.totalSongs || 0,
              processedSongs: 0,
              currentBatch: 0,
              totalBatches: payload.data?.totalBatches || 0,
              message: payload.message,
            });
			addLog('info', `[AI Enrichment] ${payload.message}`, payload.data);
            break;
          case 'enrichment_progress':
          case 'mood_progress':
            setEnrichmentStatus({
              isEnriching: true,
              totalSongs: payload.data?.totalSongs || 0,
              processedSongs: payload.data?.processedSongs || 0,
              currentBatch: payload.data?.currentBatch || 0,
              totalBatches: payload.data?.totalBatches || 0,
              message: payload.message,
            });
			const progressKey = `${payload.type}:${payload.data?.processedSongs || 0}:${payload.data?.totalSongs || 0}`;
			if (payload.data?.processedSongs && progressKey !== lastEnrichmentProgressRef.current) {
				lastEnrichmentProgressRef.current = progressKey;
				addLog('info', `[AI Enrichment] ${payload.message}`, payload.data);
				// Scanner progress is emitted after each enrichment transaction.
				// Pull that committed revision now so metadata health, smart mixes,
				// songs, and AI DJ inputs update while enrichment is still running.
				syncCommittedLibraryMutation();
			}
            break;
          case 'enrichment_complete':
          case 'mood_complete':
            setEnrichmentStatus({
              isEnriching: false,
              processedSongs: payload.data?.enrichedSongs || payload.data?.processedSongs || 0,
              message: payload.message,
            });
			addLog('success', `[AI Enrichment] ${payload.message}`, payload.data);
            void syncNow(true);
            window.dispatchEvent(new CustomEvent(payload.type, { detail: payload }));
            break;
        }
      } catch (error) {
        console.warn('Failed to parse library progress event:', error);
      }
    };

    let legacyConnection = 0;
    let revisionConnection = 0;

    const connectLegacy = async () => {
      if (disposed) return;
      const connection = ++legacyConnection;
      legacySource?.close();
      const source = new EventSource(await getEventStreamURL('/api/library/events'));
      if (disposed || connection !== legacyConnection) {
        source.close();
        return;
      }
      legacySource = source;
      source.onmessage = handleLegacyEvent;
      source.onerror = () => {
        if (source.readyState === EventSource.CLOSED && !disposed && source === legacySource) {
          legacyReconnect = setTimeout(() => { void connectLegacy(); syncCommittedLibraryMutation(); }, 3000);
        }
      };
    };

    const connectRevisionStream = async () => {
      if (disposed) return;
      const connection = ++revisionConnection;
      revisionSource?.close();
      const source = new EventSource(await getEventStreamURL(libraryV2.eventURL(useStore.getState().getLibraryRevision() ?? 0)));
      if (disposed || connection !== revisionConnection) {
        source.close();
        return;
      }
      revisionSource = source;
      source.addEventListener('library_revision', event => {
        try {
          const payload = JSON.parse((event as MessageEvent).data) as RevisionEvent;
          if (payload.revision > (useStore.getState().getLibraryRevision() ?? -1)) syncCommittedLibraryMutation();
        } catch (error) {
          console.warn('Failed to parse library revision event:', error);
        }
      });
      source.onopen = () => {
        revisionReconnects = 0;
      };
      source.onerror = () => {
        if (source.readyState !== EventSource.CLOSED || disposed || source !== revisionSource) return;
        source.close();
        revisionReconnects += 1;
        const delay = Math.min(30_000, 1000 * 2 ** Math.min(revisionReconnects, 5));
        revisionReconnect = setTimeout(() => { void connectRevisionStream(); syncCommittedLibraryMutation(); }, delay);
      };
    };

    void connectLegacy();
    void connectRevisionStream();

    return () => {
      disposed = true;
      clearTimeout(legacyReconnect);
      clearTimeout(revisionReconnect);
      if (mutationSyncTimer) clearTimeout(mutationSyncTimer);
      legacySource?.close();
      revisionSource?.close();
    };
  }, [ready]);

  return null;
};

export default LibraryEventListener;
