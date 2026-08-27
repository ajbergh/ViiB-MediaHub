import { useEffect, useRef } from 'react';
import { useStore } from '../store';
import { generateSmartMixes } from '../lib/smartMix';
import { libraryIndex } from '../lib/libraryIndex';
import { libraryV2 } from '../services/libraryV2';

const REVISION_STORAGE_KEY = 'viib-library-revision';

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
    currentBatch?: number;
    totalBatches?: number;
    enrichedSongs?: number;
  };
}

interface RevisionEvent {
  revision: number;
}

function readStoredRevision(): number {
  const value = Number(window.localStorage.getItem(REVISION_STORAGE_KEY) || '0');
  return Number.isFinite(value) && value >= 0 ? value : 0;
}

function storeRevision(revision: number): void {
  window.localStorage.setItem(REVISION_STORAGE_KEY, String(Math.max(0, revision)));
}

/**
 * Maintains the legacy progress stream and the revisioned data stream.
 * Library mutations are applied by song ID. Legacy `library_updated` events
 * prompt a coalesced delta sync and use a full refresh only while that stream
 * is unavailable.
 */
const LibraryEventListener = () => {
  const backendAvailable = useStore(state => state.backendAvailable);
  const storeRef = useRef(useStore.getState());
  const currentRevisionRef = useRef(0);
  const syncPromiseRef = useRef<Promise<void>>(Promise.resolve());
  const deltaAvailableRef = useRef(false);
	const lastEnrichmentProgressRef = useRef('');

  useEffect(() => useStore.subscribe(state => { storeRef.current = state; }), []);

  useEffect(() => {
    if (!backendAvailable) return;

    let disposed = false;
    let legacySource: EventSource | null = null;
    let revisionSource: EventSource | null = null;
    let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
    let mutationSyncTimer: ReturnType<typeof setTimeout> | null = null;
    let revisionReconnects = 0;
    const abortController = new AbortController();

    const replaceLibrary = (songs: ReturnType<typeof libraryIndex.toArray>) => {
      const smartMixes = generateSmartMixes(songs);
      useStore.setState({ songs, smartMixes });
    };

    const initializeSnapshot = async () => {
      const snapshot = await libraryV2.getSnapshot(abortController.signal);
      if (disposed) return;
      const songs = libraryIndex.initialize(snapshot.songs);
      replaceLibrary(songs);
      currentRevisionRef.current = snapshot.revision;
      storeRevision(snapshot.revision);
      deltaAvailableRef.current = true;
    };

    const applyChanges = async () => {
      const delta = await libraryV2.getChanges(currentRevisionRef.current, abortController.signal);
      if (disposed) return;
      if (delta.changes.length > 0) {
        const songs = libraryIndex.apply(delta.changes, delta.songs);
        replaceLibrary(songs);
      }
      currentRevisionRef.current = delta.revision;
      storeRevision(delta.revision);
      deltaAvailableRef.current = true;
    };

    const enqueueSync = () => {
      syncPromiseRef.current = syncPromiseRef.current
        .then(applyChanges)
        .catch(error => {
          if (!disposed && !abortController.signal.aborted) {
            deltaAvailableRef.current = false;
            console.warn('Revisioned library synchronization failed:', error);
            // Do not wait for another scanner batch to make a first-run
            // library visible if the revision endpoint is unavailable.
            void storeRef.current.refreshLibrary();
          }
        });
    };

    // Scanner batches are the most immediate signal that tracks have been
    // committed. The revision stream remains the normal efficient path, but a
    // fresh install must not stay empty while it is establishing or recovering.
    // Coalescing keeps a fast scan from issuing one sync request per batch.
    const syncCommittedLibraryMutation = () => {
      if (mutationSyncTimer) return;
      mutationSyncTimer = setTimeout(() => {
        mutationSyncTimer = null;
        if (disposed) return;
        if (deltaAvailableRef.current) {
          enqueueSync();
        } else {
          void storeRef.current.refreshLibrary();
        }
      }, 100);
    };

    const handleLegacyEvent = (event: MessageEvent) => {
      try {
        const payload = JSON.parse(event.data) as LegacyLibraryEvent;
        const { setScanning, setScanProgress, setEnrichmentStatus, refreshLibrary, addLog } = storeRef.current;
        switch (payload.type) {
          case 'scan_started':
            setScanning(true);
            setScanProgress(payload.message);
            break;
          case 'scan_progress':
            setScanProgress(payload.message);
            break;
          case 'scan_complete':
            setScanning(false);
            setScanProgress('');
            if (deltaAvailableRef.current) enqueueSync();
            else refreshLibrary();
            break;
          case 'library_updated':
            // A local scan emits this after each committed batch. Prefer a
            // revision delta, but use the legacy full refresh if its stream is
            // not connected yet so first-run libraries populate immediately.
            if (storeRef.current.isScanning && payload.message) {
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
            if (deltaAvailableRef.current) enqueueSync();
            else refreshLibrary();
            window.dispatchEvent(new CustomEvent(payload.type, { detail: payload }));
            break;
        }
      } catch (error) {
        console.warn('Failed to parse library progress event:', error);
      }
    };

    const connectLegacy = () => {
      if (disposed) return;
      legacySource?.close();
      legacySource = new EventSource('/api/library/events');
      legacySource.onmessage = handleLegacyEvent;
      legacySource.onerror = () => {
        if (legacySource?.readyState === EventSource.CLOSED && !disposed) {
          reconnectTimer = setTimeout(connectLegacy, 3000);
        }
      };
    };

    const connectRevisionStream = () => {
      if (disposed) return;
      revisionSource?.close();
      revisionSource = new EventSource(libraryV2.eventURL(currentRevisionRef.current));
      revisionSource.addEventListener('library_revision', event => {
        try {
          const payload = JSON.parse((event as MessageEvent).data) as RevisionEvent;
          if (payload.revision > currentRevisionRef.current) enqueueSync();
        } catch (error) {
          console.warn('Failed to parse library revision event:', error);
        }
      });
      revisionSource.onopen = () => {
        revisionReconnects = 0;
        deltaAvailableRef.current = true;
      };
      revisionSource.onerror = () => {
        if (revisionSource?.readyState !== EventSource.CLOSED || disposed) return;
        deltaAvailableRef.current = false;
        revisionSource.close();
        revisionReconnects += 1;
        const delay = Math.min(30_000, 1000 * 2 ** Math.min(revisionReconnects, 5));
        reconnectTimer = setTimeout(connectRevisionStream, delay);
      };
    };

    const start = async () => {
      currentRevisionRef.current = readStoredRevision();
      try {
        // A paginated snapshot establishes a consistent local index. Subsequent
        // updates use only the durable change log.
        await initializeSnapshot();
      } catch (error) {
        if (!abortController.signal.aborted) {
          console.warn('Versioned library snapshot unavailable; retaining legacy library state:', error);
          libraryIndex.initialize(storeRef.current.songs);
          deltaAvailableRef.current = false;
        }
      }
      if (disposed) return;
      connectLegacy();
      connectRevisionStream();
    };

    void start();

    return () => {
      disposed = true;
      abortController.abort();
      if (reconnectTimer) clearTimeout(reconnectTimer);
      if (mutationSyncTimer) clearTimeout(mutationSyncTimer);
      legacySource?.close();
      revisionSource?.close();
    };
  }, [backendAvailable]);

  return null;
};

export default LibraryEventListener;
