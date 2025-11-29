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
  type: 'scan_started' | 'scan_complete' | 'scan_progress';
  message: string;
  newSongs?: number;
  totalSongs?: number;
}

const LibraryEventListener = () => {
  const eventSourceRef = useRef<EventSource | null>(null);
  const reconnectTimeoutRef = useRef<NodeJS.Timeout | null>(null);
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

      const { setScanning, setScanProgress, refreshLibrary } = storeRef.current;

      switch (libraryEvent.type) {
        case 'scan_started':
          setScanning(true);
          setScanProgress(libraryEvent.message);
          break;

        case 'scan_complete':
          setScanning(false);
          setScanProgress('');
          // Always refresh the library when scan completes
          console.log(`🎵 Scan complete (${libraryEvent.newSongs || 0} new songs), refreshing library...`);
          refreshLibrary();
          break;

        case 'scan_progress':
          setScanProgress(libraryEvent.message);
          break;
      }
    } catch (error) {
      console.error('Failed to parse library event:', error);
    }
  }, []);

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
        console.log('✅ Library events SSE connected');
      };

      eventSource.onmessage = handleEvent;

      eventSource.onerror = () => {
        console.warn('Library events SSE error, reconnecting in 3s...');
        eventSource.close();
        eventSourceRef.current = null;
        // Reconnect after 3 seconds
        reconnectTimeoutRef.current = setTimeout(connect, 3000);
      };
    };

    connect();

    return () => {
      console.log('🔌 Closing library events SSE connection');
      if (reconnectTimeoutRef.current) {
        clearTimeout(reconnectTimeoutRef.current);
        reconnectTimeoutRef.current = null;
      }
      if (eventSourceRef.current) {
        eventSourceRef.current.close();
        eventSourceRef.current = null;
      }
    };
  }, [backendAvailable, handleEvent]);

  // This component doesn't render anything
  return null;
};

export default LibraryEventListener;
