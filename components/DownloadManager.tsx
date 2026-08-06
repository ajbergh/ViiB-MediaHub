/**
 * ViiB MediaHub - Download Manager Component
 * 
 * Background component managing Spotify download state and progress.
 * Does not render any UI - only manages download count in global state.
 * 
 * Features:
 * - Fetches initial download list from backend on mount
 * - Maintains SSE connection for real-time progress updates
 * - Polling fallback when SSE connection fails
 * - Updates global download count for sidebar badge
 * 
 * The actual download progress UI is displayed on the Downloads page.
 * This component ensures download state is synchronized globally.
 * 
 * @module DownloadManager
 */

import React, { useEffect, useRef, useCallback } from 'react';
import api from '../services/api';
import { useStore } from '../store';

interface DownloadProgress {
  downloadId: string;
  progress: number;
  status: string;
  error?: string;
}

const DownloadManager = () => {
  const eventSourceRef = useRef<EventSource | null>(null);
  const refreshTimeoutRef = useRef<number | null>(null);
  const requestSequenceRef = useRef(0);
  const setDownloadCount = useStore(state => state.setDownloadCount);

  // Fetch the exact active count rather than deriving it from the paginated
  // download-history response, which can omit active rows in a large history.
  const fetchActiveDownloadCount = useCallback(async () => {
    const requestSequence = ++requestSequenceRef.current;
    try {
      const { count } = await api.getActiveDownloadCount();
      if (requestSequence === requestSequenceRef.current) {
        setDownloadCount(count);
      }
    } catch (error) {
      console.error('Failed to fetch active download count:', error);
    }
  }, [setDownloadCount]);

  const scheduleCountRefresh = useCallback(() => {
    if (refreshTimeoutRef.current !== null) return;
    refreshTimeoutRef.current = window.setTimeout(() => {
      refreshTimeoutRef.current = null;
      void fetchActiveDownloadCount();
    }, 100);
  }, [fetchActiveDownloadCount]);

  // Reconcile at startup and periodically so reconnects or dropped SSE events
  // cannot leave the navigation badge stale.
  useEffect(() => {
    void fetchActiveDownloadCount();
    const interval = window.setInterval(fetchActiveDownloadCount, 15_000);
    return () => window.clearInterval(interval);
  }, [fetchActiveDownloadCount]);

  // Connect to SSE for real-time updates
  useEffect(() => {
    const eventSource = new EventSource('/api/spotify/downloads/events');
    eventSourceRef.current = eventSource;

    eventSource.onopen = () => {
      void fetchActiveDownloadCount();
    };

    eventSource.onmessage = (event) => {
      try {
        const progress: DownloadProgress = JSON.parse(event.data);
        if (progress.status !== 'downloading') {
          scheduleCountRefresh();
        }
      } catch (error) {
        console.error('Failed to parse SSE message:', error);
      }
    };

    eventSource.onerror = () => {
      console.warn('DownloadManager SSE connection error; EventSource will reconnect');
      scheduleCountRefresh();
    };

    return () => {
      eventSource.close();
      eventSourceRef.current = null;
      if (refreshTimeoutRef.current !== null) {
        window.clearTimeout(refreshTimeoutRef.current);
        refreshTimeoutRef.current = null;
      }
    };
  }, [fetchActiveDownloadCount, scheduleCountRefresh]);

  // This component doesn't render anything - it just manages the download count
  return null;
};

export default DownloadManager;
