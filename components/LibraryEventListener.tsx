/**
 * ViiB MediaHub - Library Event Listener Component
 * 
 * Background component maintaining SSE connection for library updates.
 * 
 * Events:
 * - scan_started: Library scan initiated
 * - scan_progress: Scan progress update
 * - scan_complete: Scan finished with new/removed song counts
 * 
 * Automatically refreshes library state when scan completes,
 * ensuring UI stays synchronized without manual reload.
 * Includes reconnection logic for dropped SSE connections.
 * 
 * @module LibraryEventListener
 */

import { useEffect, useRef, useCallback } from 'react';
import { useStore } from '../store';

/**
 * LibraryEventListener - Listens for library events via SSE and refreshes the library
 * 
 * This component connects to the backend's /api/library/events SSE endpoint
 * and listens for events like scan_complete. When a scan completes, it
 * automatically refreshes the library data so the UI updates without manual reload.
 */
interface LibraryEvent {
  type: 'scan_started' | 'scan_complete' | 'scan_progress' | 'enrichment_started' | 'enrichment_progress' | 'enrichment_complete' | 'mood_started' | 'mood_progress' | 'mood_complete' | 'library_updated';
  message: string;
  newSongs?: number;
  removedSongs?: number;
  totalSongs?: number;
  data?: {
    totalSongs?: number;
    processedSongs?: number;
    currentBatch?: number;
    totalBatches?: number;
    enrichedSongs?: number;
  };
}

const LibraryEventListener = () => {
  const eventSourceRef = useRef<EventSource | null>(null);
  const reconnectTimeoutRef = useRef<NodeJS.Timeout | null>(null);
  const refreshDebounceRef = useRef<NodeJS.Timeout | null>(null);
  const lastRefreshTimeRef = useRef<number>(0);
  const backendAvailable = useStore(state => state.backendAvailable);
  
  // Use refs for callbacks to prevent SSE reconnection on store updates
  const storeRef = useRef(useStore.getState());
  useEffect(() => {
    // Subscribe to store changes but keep ref updated
    return useStore.subscribe(state => {
      storeRef.current = state;
    });
  }, []);

  const handleEvent = useCallback((event: MessageEvent) => {
    try {
      const libraryEvent: LibraryEvent = JSON.parse(event.data);
      console.log('📥 Library event received:', libraryEvent);

      const { setScanning, setScanProgress, refreshLibrary, setEnrichmentStatus } = storeRef.current;

      switch (libraryEvent.type) {
        case 'scan_started':
          setScanning(true);
          setScanProgress(libraryEvent.message);
          break;

        case 'scan_complete':
          setScanning(false);
          setScanProgress('');
          // Cancel any pending debounced refresh
          if (refreshDebounceRef.current) {
            clearTimeout(refreshDebounceRef.current);
            refreshDebounceRef.current = null;
          }
          // Always refresh the library when scan completes - immediate, not debounced
          const added = libraryEvent.newSongs || 0;
          const removed = libraryEvent.removedSongs || 0;
          console.log(`🎵 Scan complete (${added} new, ${removed} removed), refreshing library...`);
          refreshLibrary();
          break;

        case 'scan_progress':
          setScanProgress(libraryEvent.message);
          break;

        case 'library_updated':
          console.log(`📚 Library updated: ${libraryEvent.message}`);
          // Use throttle + debounce: refresh at most every 2s during scan, plus final debounced refresh
          const now = Date.now();
          const timeSinceLastRefresh = now - lastRefreshTimeRef.current;
          
          // Clear any pending debounced refresh
          if (refreshDebounceRef.current) {
            clearTimeout(refreshDebounceRef.current);
          }
          
          // Throttle: if more than 2s since last refresh, refresh immediately
          if (timeSinceLastRefresh >= 2000) {
            console.log(`📚 Throttled refresh triggered (${timeSinceLastRefresh}ms since last)`);
            lastRefreshTimeRef.current = now;
            refreshLibrary();
          }
          
          // Debounce: schedule a refresh for 500ms after last event (for final update)
          refreshDebounceRef.current = setTimeout(() => {
            console.log(`📚 Debounced refresh triggered`);
            lastRefreshTimeRef.current = Date.now();
            refreshLibrary();
            refreshDebounceRef.current = null;
          }, 500);
          
          // Dispatch window event for components that need to know (e.g., Genres page)
          window.dispatchEvent(new CustomEvent('library_updated', { detail: libraryEvent }));
          break;

        case 'enrichment_started':
          console.log(`✨ Enrichment started: ${libraryEvent.message}`, libraryEvent.data);
          setEnrichmentStatus({
            isEnriching: true,
            totalSongs: libraryEvent.data?.totalSongs || 0,
            processedSongs: 0,
            currentBatch: 0,
            totalBatches: libraryEvent.data?.totalBatches || 0,
            message: libraryEvent.message,
          });
          console.log(`✨ Called setEnrichmentStatus with isEnriching: true`);
          break;

        case 'enrichment_progress':
          console.log(`✨ Enrichment progress: ${libraryEvent.message}`);
          setEnrichmentStatus({
            isEnriching: true,
            totalSongs: libraryEvent.data?.totalSongs || 0,
            processedSongs: libraryEvent.data?.processedSongs || 0,
            currentBatch: libraryEvent.data?.currentBatch || 0,
            totalBatches: libraryEvent.data?.totalBatches || 0,
            message: libraryEvent.message,
          });
          break;

        case 'enrichment_complete':
          console.log(`✨ Enrichment complete: ${libraryEvent.message}`);
          setEnrichmentStatus({
            isEnriching: false,
            processedSongs: libraryEvent.data?.enrichedSongs || 0,
            message: libraryEvent.message,
          });
          refreshLibrary();
          // Dispatch window event for components that need to refresh (e.g., Genres page)
          // Genre enrichment updates genre_stats table, so UI should refresh
          window.dispatchEvent(new CustomEvent('enrichment_complete', { detail: libraryEvent }));
          
          // Clear enrichment status after a delay
          setTimeout(() => {
            setEnrichmentStatus({
              isEnriching: false,
              totalSongs: 0,
              processedSongs: 0,
              currentBatch: 0,
              totalBatches: 0,
              message: '',
            });
          }, 5000);
          break;

        // Mood analysis events - use same enrichmentStatus for sidebar display
        case 'mood_started':
          console.log(`🎭 Mood analysis started: ${libraryEvent.message}`, libraryEvent.data);
          setEnrichmentStatus({
            isEnriching: true,
            totalSongs: libraryEvent.data?.totalSongs || 0,
            processedSongs: 0,
            currentBatch: 0,
            totalBatches: libraryEvent.data?.totalBatches || 0,
            message: libraryEvent.message,
          });
          break;

        case 'mood_progress':
          console.log(`🎭 Mood analysis progress: ${libraryEvent.message}`);
          setEnrichmentStatus({
            isEnriching: true,
            totalSongs: libraryEvent.data?.totalSongs || 0,
            processedSongs: libraryEvent.data?.processedSongs || 0,
            currentBatch: libraryEvent.data?.currentBatch || 0,
            totalBatches: libraryEvent.data?.totalBatches || 0,
            message: libraryEvent.message,
          });
          break;

        case 'mood_complete':
          console.log(`🎭 Mood analysis complete: ${libraryEvent.message}`);
          setEnrichmentStatus({
            isEnriching: false,
            processedSongs: libraryEvent.data?.processedSongs || 0,
            message: libraryEvent.message,
          });
          refreshLibrary();
          // Dispatch window event for components that need to know
          window.dispatchEvent(new CustomEvent('mood_complete', { detail: libraryEvent }));
          
          // Clear enrichment status after a delay
          setTimeout(() => {
            setEnrichmentStatus({
              isEnriching: false,
              totalSongs: 0,
              processedSongs: 0,
              currentBatch: 0,
              totalBatches: 0,
              message: '',
            });
          }, 5000);
          break;
      }
    } catch (error) {
      console.error('Failed to parse library event:', error);
    }
  }, []);

  // Store handleEvent in a ref for stability
  const handleEventRef = useRef(handleEvent);
  handleEventRef.current = handleEvent;

  // Track if this is initial connection vs reconnection
  const isReconnectRef = useRef(false);
  // Count consecutive reconnects to surface persistent failures
  const reconnectCountRef = useRef(0);

  useEffect(() => {
    // Wait for backend to be available before connecting
    if (!backendAvailable) {
      return;
    }

    // Don't reconnect if already connected
    if (eventSourceRef.current && eventSourceRef.current.readyState !== EventSource.CLOSED) {
      return;
    }
    
    const connect = () => {
      const eventSource = new EventSource('/api/library/events');
      eventSourceRef.current = eventSource;

      eventSource.onopen = () => {
        // Reset reconnect counter on successful connection
        reconnectCountRef.current = 0;

        // Always check scan status when SSE connects (even on initial connect)
        // This handles the case where the scan started before SSE connected
        const checkScanStatus = async () => {
          try {
            const { getScanStatus } = await import('../services/backendService').then(m => m.backendService);
            const status = await getScanStatus();
            const { setScanning, setScanProgress, refreshLibrary, isScanning } = storeRef.current;
            
            if (status.scanning) {
              // Scan is in progress - make sure UI reflects this
              setScanning(true);
              if (status.progress) {
                setScanProgress(status.progress);
              }
            } else if (isReconnectRef.current) {
              // Only refresh on reconnect if not initial connection
              // (initial connection already loaded library in initLibrary)
              setScanning(false);
              setScanProgress('');
              refreshLibrary();
            }
          } catch (e) {
            console.warn('Could not check scan status after SSE connect:', e);
          }
        };
        checkScanStatus();
        
        // Mark that next connect will be a reconnect
        isReconnectRef.current = true;
      };

      eventSource.onmessage = (event) => handleEventRef.current(event);

      eventSource.onerror = () => {
        // Only reconnect if the connection is actually closed
        // SSE can fire error events transiently without the connection being lost
        if (eventSource.readyState === EventSource.CLOSED) {
          reconnectCountRef.current += 1;
          // Only warn after repeated failures to avoid console spam on normal reconnects
          if (reconnectCountRef.current >= 3) {
            console.warn(`Library events SSE: ${reconnectCountRef.current} consecutive reconnects — backend may be unavailable`);
          }
          eventSource.close();
          eventSourceRef.current = null;
          // Reconnect after 3 seconds
          reconnectTimeoutRef.current = setTimeout(connect, 3000);
        }
        // If CONNECTING or OPEN, ignore — connection is recovering or still valid
      };
    };

    connect();

    // Safety fallback: Check scan status periodically to handle missed events
    // This catches cases where scan_complete was missed due to SSE disconnect
    const scanStatusInterval = setInterval(async () => {
      const { isScanning, setScanning, setScanProgress, refreshLibrary } = storeRef.current;
      
      // Only check if we think a scan is in progress
      if (!isScanning) return;
      
      try {
        const { getScanStatus } = await import('../services/backendService').then(m => m.backendService);
        const status = await getScanStatus();
        
        if (!status.scanning) {
          setScanning(false);
          setScanProgress('');
          refreshLibrary();
        }
      } catch {
        // Silently ignore - we'll try again on the next interval
      }
    }, 5000); // Check every 5 seconds

    return () => {
      clearInterval(scanStatusInterval);
      if (reconnectTimeoutRef.current) {
        clearTimeout(reconnectTimeoutRef.current);
        reconnectTimeoutRef.current = null;
      }
      if (eventSourceRef.current) {
        eventSourceRef.current.close();
        eventSourceRef.current = null;
      }
    };
  }, [backendAvailable]); // handleEvent is stable via useCallback with empty deps

  // This component doesn't render anything
  return null;
};

export default LibraryEventListener;
