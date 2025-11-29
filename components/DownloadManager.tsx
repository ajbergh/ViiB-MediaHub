import React, { useEffect, useState, useRef, useCallback } from 'react';
import api, { ApiSpotifyDownload } from '../services/api';
import { useStore } from '../store';

interface DownloadProgress {
  downloadId: string;
  progress: number;
  status: string;
  error?: string;
}

const DownloadManager = () => {
  const [downloads, setDownloads] = useState<ApiSpotifyDownload[]>([]);
  const [hasError, setHasError] = useState(false);
  const eventSourceRef = useRef<EventSource | null>(null);
  const pollIntervalRef = useRef<NodeJS.Timeout | null>(null);
  const { setDownloadCount } = useStore();

  // Update global download count whenever downloads change
  useEffect(() => {
    const activeCount = downloads.filter(d => d.status === 'downloading' || d.status === 'queued').length;
    setDownloadCount(activeCount);
  }, [downloads, setDownloadCount]);

  // Fetch downloads function - memoized for reuse
  const fetchDownloads = useCallback(async () => {
    try {
      const data = await api.getDownloads();
      setDownloads(data || []);
      setHasError(false);
    } catch (error) {
      console.error('Failed to fetch downloads:', error);
      setHasError(true);
    }
  }, []);

  // Fetch initial downloads
  useEffect(() => {
    fetchDownloads();
  }, [fetchDownloads]);

  // Polling fallback - ensures count stays updated even if SSE has issues
  useEffect(() => {
    const hasActiveDownloads = downloads.some(d => d.status === 'downloading' || d.status === 'queued');
    
    if (hasActiveDownloads) {
      // Poll every 2 seconds when there are active downloads
      pollIntervalRef.current = setInterval(fetchDownloads, 2000);
    }

    return () => {
      if (pollIntervalRef.current) {
        clearInterval(pollIntervalRef.current);
        pollIntervalRef.current = null;
      }
    };
  }, [downloads, fetchDownloads]);

  // Connect to SSE for real-time updates
  useEffect(() => {
    if (hasError) return;

    const eventSource = new EventSource('/api/spotify/downloads/events');
    eventSourceRef.current = eventSource;

    eventSource.onmessage = (event) => {
      try {
        const progress: DownloadProgress = JSON.parse(event.data);
        
        setDownloads(prev => {
          const existing = prev.find(d => d.id === progress.downloadId);
          if (existing) {
            return prev.map(d => 
              d.id === progress.downloadId 
                ? { ...d, progress: progress.progress, status: progress.status as any, errorMessage: progress.error }
                : d
            );
          } else {
            // New download detected, fetch all
            fetchDownloads();
            return prev;
          }
        });
      } catch (error) {
        console.error('Failed to parse SSE message:', error);
      }
    };

    eventSource.onerror = () => {
      console.warn('DownloadManager SSE connection error, will rely on polling');
      eventSource.close();
    };

    return () => {
      eventSource.close();
    };
  }, [hasError, fetchDownloads]);

  // This component doesn't render anything - it just manages the download count
  return null;
};

export default DownloadManager;
