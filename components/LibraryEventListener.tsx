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

  useEffect(() => {
    // Wait for backend to be available before connecting
    if (!backendAvailable) {
      console.log('📡 LibraryEventListener: Waiting for backend...');
      return;
    }

    // Don't reconnect if already connected
    if (eventSourceRef.current && eventSourceRef.current.readyState !== EventSource.CLOSED) {
      console.log('📡 LibraryEventListener: Already connected, skipping');
      return;
    }

    console.log('📡 Connecting to library events SSE...');
    
    const connect = () => {
      const eventSource = new EventSource('/api/library/events');
      eventSourceRef.current = eventSource;

      eventSource.onopen = () => {
        console.log('✅ Library events SSE connected', isReconnectRef.current ? '(reconnect)' : '(initial)');
        
        // Only check scan status on RECONNECTS, not initial connection
        // Initial connection should trust the UI state set by initLibrary (which sets isScanning=true for startup scan)
        if (!isReconnectRef.current) {
          console.log('📡 Initial SSE connection - not resetting scan state');
          isReconnectRef.current = true; // Mark that next connect will be a reconnect
          return;
        }
        
        // After reconnecting, check if a scan is in progress or just completed
        // This handles the case where we missed events during disconnect
        const checkScanStatus = async () => {
          try {
            const { getScanStatus } = await import('../services/backendService').then(m => m.backendService);
            const status = await getScanStatus();
            const { setScanning, setScanProgress, refreshLibrary } = storeRef.current;
            
            if (status.scanning) {
              console.log('🔍 SSE reconnected - scan in progress, updating UI state...');
              setScanning(true);
              setScanProgress(status.progress || 'Scanning...');
            } else {
              // Scan may have just finished while we were disconnected
              // Refresh the library to get latest state
              console.log('🔍 SSE reconnected - no scan in progress, refreshing library...');
              setScanning(false);
              setScanProgress('');
              refreshLibrary();
            }
          } catch (e) {
            console.warn('Could not check scan status after SSE reconnect:', e);
          }
        };
        checkScanStatus();
      };

      eventSource.onmessage = (event) => handleEventRef.current(event);

      eventSource.onerror = (e) => {
        // Only reconnect if the connection is actually closed
        // SSE can fire error events transiently without the connection being lost
        if (eventSource.readyState === EventSource.CLOSED) {
          console.warn('Library events SSE connection closed, reconnecting in 3s...');
          eventSource.close();
          eventSourceRef.current = null;
          // Reconnect after 3 seconds
          reconnectTimeoutRef.current = setTimeout(connect, 3000);
        } else if (eventSource.readyState === EventSource.CONNECTING) {
          console.log('📡 SSE is reconnecting...');
        }
        // If OPEN, ignore the error - connection is still valid
      };
    };

    connect();

    // Safety fallback: Check scan status periodically to handle missed events
    // This catches cases where scan_complete was missed due to SSE disconnect
    const scanStatusInterval = setInterval(async () => {
      const { isScanning, setScanning, setScanProgress, refreshLibrary } = storeRef.current;
      
      console.log('⏰ 5-second scan status check, isScanning=', isScanning);
      
      // Only check if we think a scan is in progress
      if (!isScanning) return;
      
      try {
        const { getScanStatus } = await import('../services/backendService').then(m => m.backendService);
        const status = await getScanStatus();
        console.log('⏰ 5-second poll result:', status);
        
        if (!status.scanning) {
          console.log('🔍 Scan status poll: Backend says scan completed, resetting UI');
          setScanning(false);
          setScanProgress('');
          refreshLibrary();
        }
      } catch (e) {
        console.error('⏰ 5-second poll error:', e);
        // Silently ignore - we'll try again
      }
    }, 5000); // Check every 5 seconds

    return () => {
      console.log('🔌 Closing library events SSE connection');
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
