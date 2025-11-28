import React, { useEffect, useState, useRef } from 'react';
import api, { ApiSpotifyDownload } from '../services/api';
import { useStore } from '../store';

interface DownloadProgress {
  id: string;
  progress: number;
  status: string;
  errorMessage?: string;
}

const DownloadManager = () => {
  const [downloads, setDownloads] = useState<ApiSpotifyDownload[]>([]);
  const [isExpanded, setIsExpanded] = useState(false);
  const [hasError, setHasError] = useState(false);
  const eventSourceRef = useRef<EventSource | null>(null);
  const { setDownloadCount } = useStore();

  // Update global download count whenever downloads change
  useEffect(() => {
    const activeCount = downloads.filter(d => d.status === 'downloading' || d.status === 'queued').length;
    setDownloadCount(activeCount);
  }, [downloads, setDownloadCount]);

  // Fetch initial downloads
  useEffect(() => {
    const fetchDownloads = async () => {
      try {
        const data = await api.getDownloads();
        setDownloads(data);
        setHasError(false);
      } catch (error) {
        console.error('Failed to fetch downloads:', error);
        setHasError(true);
      }
    };

    fetchDownloads();
  }, []);

  // Connect to SSE for real-time updates
  useEffect(() => {
    if (hasError) return; // Don't connect if we had an error fetching downloads

    const eventSource = new EventSource('/api/spotify/downloads/events');
    eventSourceRef.current = eventSource;

    eventSource.onmessage = (event) => {
      try {
        const progress: DownloadProgress = JSON.parse(event.data);
        
        setDownloads(prev => {
          const existing = prev.find(d => d.id === progress.id);
          if (existing) {
            return prev.map(d => 
              d.id === progress.id 
                ? { ...d, progress: progress.progress, status: progress.status as any, errorMessage: progress.errorMessage }
                : d
            );
          } else {
            // New download detected, fetch all to be safe and get full details
            // Or we could try to construct it if we had all data, but we don't have title/artist in the progress event usually
            // For now, let's just trigger a refetch
            api.getDownloads().then(setDownloads).catch(console.error);
            return prev;
          }
        });
      } catch (error) {
        console.error('Failed to parse SSE message:', error);
      }
    };

    eventSource.onerror = (error) => {
      console.error('SSE connection error:', error);
      eventSource.close();
    };

    return () => {
      eventSource.close();
    };
  }, [hasError]);

  const handleDelete = async (id: string) => {
    try {
      await api.deleteDownload(id);
      setDownloads(prev => prev.filter(d => d.id !== id));
    } catch (error) {
      console.error('Failed to delete download:', error);
    }
  };

  const activeDownloads = downloads.filter(d => d.status === 'downloading' || d.status === 'queued');
  const completedDownloads = downloads.filter(d => d.status === 'completed');
  const failedDownloads = downloads.filter(d => d.status === 'failed');

  // Don't render if there's an error or no downloads
  if (hasError || downloads.length === 0) return null;

  // We only want the logic for the global store/sidebar badge, not the floating window
  return null;
};

interface DownloadItemProps {
  download: ApiSpotifyDownload;
  onDelete: (id: string) => void;
}

const DownloadItem: React.FC<DownloadItemProps> = ({ download, onDelete }) => {
  const getStatusColor = () => {
    switch (download.status) {
      case 'downloading': return 'text-brand';
      case 'completed': return 'text-green-400';
      case 'failed': return 'text-red-400';
      default: return 'text-text-muted';
    }
  };

  const getStatusIcon = () => {
    switch (download.status) {
      case 'downloading':
        return (
          <svg className="w-4 h-4 animate-spin" fill="none" viewBox="0 0 24 24">
            <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4"></circle>
            <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
          </svg>
        );
      case 'completed':
        return (
          <svg className="w-4 h-4" fill="currentColor" viewBox="0 0 20 20">
            <path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clipRule="evenodd" />
          </svg>
        );
      case 'failed':
        return (
          <svg className="w-4 h-4" fill="currentColor" viewBox="0 0 20 20">
            <path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.707 7.293a1 1 0 00-1.414 1.414L8.586 10l-1.293 1.293a1 1 0 101.414 1.414L10 11.414l1.293 1.293a1 1 0 001.414-1.414L11.414 10l1.293-1.293a1 1 0 00-1.414-1.414L10 8.586 8.707 7.293z" clipRule="evenodd" />
          </svg>
        );
      default:
        return (
          <svg className="w-4 h-4" fill="currentColor" viewBox="0 0 20 20">
            <path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm1-12a1 1 0 10-2 0v4a1 1 0 00.293.707l2.828 2.829a1 1 0 101.415-1.415L11 9.586V6z" clipRule="evenodd" />
          </svg>
        );
    }
  };

  return (
    <div className="bg-surface-3 rounded-lg p-3">
      <div className="flex items-start gap-3">
        <div className={`mt-1 ${getStatusColor()}`}>
          {getStatusIcon()}
        </div>
        <div className="flex-1 min-w-0">
          <p className="font-medium text-white truncate">{download.title}</p>
          <p className="text-sm text-text-muted truncate">{download.artist}</p>
          {download.album && <p className="text-xs text-text-muted truncate">{download.album}</p>}
          
          {/* Progress Bar */}
          {download.status === 'downloading' && (
            <div className="mt-2">
              <div className="h-1 bg-surface-1 rounded-full overflow-hidden">
                <div 
                  className="h-full bg-brand transition-all duration-300"
                  style={{ width: `${download.progress}%` }}
                />
              </div>
              <p className="text-xs text-text-muted mt-1">{download.progress.toFixed(0)}%</p>
            </div>
          )}

          {/* Error Message */}
          {download.status === 'failed' && download.errorMessage && (
            <p className="text-xs text-red-400 mt-1">{download.errorMessage}</p>
          )}
        </div>

        {/* Delete Button */}
        <button
          onClick={() => onDelete(download.id)}
          className="p-1 hover:bg-surface-2 rounded transition-colors"
          title={download.status === 'downloading' ? "Cancel" : "Remove"}
        >
          <svg className={`w-4 h-4 ${download.status === 'downloading' ? 'text-red-400' : 'text-text-muted hover:text-white'}`} fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
          </svg>
        </button>
      </div>
    </div>
  );
};

export default DownloadManager;
