import React, { useEffect, useState, useRef, useCallback } from 'react';
import { Download, Loader2, CheckCircle, XCircle, Clock, Trash2, Music, RefreshCw, RotateCcw } from 'lucide-react';
import api, { ApiSpotifyDownload } from '../services/api';

export const Downloads: React.FC = () => {
  const [downloads, setDownloads] = useState<ApiSpotifyDownload[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [hasError, setHasError] = useState(false);
  const [filter, setFilter] = useState<'all' | 'active' | 'completed' | 'failed'>('all');
  const eventSourceRef = useRef<EventSource | null>(null);
  const pollIntervalRef = useRef<NodeJS.Timeout | null>(null);

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
    const initialFetch = async () => {
      await fetchDownloads();
      setIsLoading(false);
    };
    initialFetch();
  }, [fetchDownloads]);

  // Polling fallback for SSE - ensures UI stays updated even if SSE fails
  useEffect(() => {
    // Poll every 3 seconds when there are active downloads
    const hasActiveDownloads = downloads.some(d => d.status === 'downloading' || d.status === 'queued');
    
    if (hasActiveDownloads && !isLoading) {
      pollIntervalRef.current = setInterval(fetchDownloads, 3000);
    }

    return () => {
      if (pollIntervalRef.current) {
        clearInterval(pollIntervalRef.current);
        pollIntervalRef.current = null;
      }
    };
  }, [downloads, isLoading, fetchDownloads]);

  // Connect to SSE for real-time updates
  useEffect(() => {
    // Don't connect if still loading or if we had an error
    if (isLoading || hasError) return;

    let eventSource: EventSource | null = null;
    let reconnectTimeout: NodeJS.Timeout | null = null;
    
    const connect = () => {
      try {
        eventSource = new EventSource('/api/spotify/downloads/events');
        eventSourceRef.current = eventSource;

        eventSource.onmessage = (event) => {
          try {
            const progress: { downloadId: string; progress: number; status: string; error?: string } = JSON.parse(event.data);
            
            // Update the matching download
            setDownloads(prev => {
              const existing = prev.find(d => d.id === progress.downloadId);
              if (existing) {
                return prev.map(d => 
                  d.id === progress.downloadId 
                    ? { ...d, progress: progress.progress, status: progress.status as any, errorMessage: progress.error }
                    : d
                );
              } else {
                // If download doesn't exist yet, refetch all downloads
                fetchDownloads();
                return prev;
              }
            });
          } catch (error) {
            console.error('Failed to parse SSE message:', error);
          }
        };

        eventSource.onerror = () => {
          console.warn('SSE connection error, will reconnect...');
          if (eventSource) {
            eventSource.close();
          }
          // Reconnect after 5 seconds
          reconnectTimeout = setTimeout(connect, 5000);
        };
      } catch (error) {
        console.error('Failed to create SSE connection:', error);
      }
    };

    connect();

    return () => {
      if (reconnectTimeout) {
        clearTimeout(reconnectTimeout);
      }
      if (eventSource) {
        eventSource.close();
      }
    };
  }, [isLoading, hasError, fetchDownloads]);

  const handleDelete = async (id: string) => {
    try {
      await api.deleteDownload(id);
      setDownloads(prev => prev.filter(d => d.id !== id));
    } catch (error) {
      console.error('Failed to delete download:', error);
    }
  };

  const handleRetry = async (id: string) => {
    try {
      await api.retryDownload(id);
      // Update local state immediately
      setDownloads(prev => prev.map(d => 
        d.id === id ? { ...d, status: 'queued' as const, progress: 0, errorMessage: undefined } : d
      ));
    } catch (error) {
      console.error('Failed to retry download:', error);
    }
  };

  const handleClearQueue = async () => {
    if (!window.confirm('Are you sure you want to clear all queued downloads?')) return;
    
    const queued = downloads.filter(d => d.status === 'queued');
    // Delete in parallel
    await Promise.all(queued.map(d => handleDelete(d.id)));
  };

  const handleClearCompleted = async () => {
    if (!window.confirm('Are you sure you want to clear all completed downloads? (Files will not be deleted)')) return;
    
    try {
      await api.clearCompletedDownloads();
      setDownloads(prev => prev.filter(d => d.status !== 'completed'));
    } catch (error) {
      console.error('Failed to clear completed downloads:', error);
    }
  };

  const handleRetryAllFailed = async () => {
    const failed = downloads.filter(d => d.status === 'failed');
    // Retry in parallel
    await Promise.all(failed.map(d => handleRetry(d.id)));
  };

  // Sort: downloading first, then queued, then failed, then completed
  // Within each group, sort by addedAt (oldest first for active, newest first for completed)
  const filteredDownloads = downloads.filter(d => {
    if (filter === 'all') return true;
    if (filter === 'active') return d.status === 'downloading' || d.status === 'queued';
    if (filter === 'completed') return d.status === 'completed';
    if (filter === 'failed') return d.status === 'failed';
    return true;
  }).sort((a, b) => {
    // Status priority: downloading > queued > failed > completed
    const statusOrder = { downloading: 0, queued: 1, failed: 2, completed: 3 };
    const statusDiff = statusOrder[a.status] - statusOrder[b.status];
    if (statusDiff !== 0) return statusDiff;
    
    // For active (downloading/queued), oldest first (FIFO)
    if (a.status === 'downloading' || a.status === 'queued') {
      return a.addedAt - b.addedAt;
    }
    // For completed/failed, newest first
    return b.addedAt - a.addedAt;
  });

  const stats = {
    active: downloads.filter(d => d.status === 'downloading' || d.status === 'queued').length,
    completed: downloads.filter(d => d.status === 'completed').length,
    failed: downloads.filter(d => d.status === 'failed').length,
  };

  if (isLoading) {
    return (
      <div className="p-8 pb-32 flex flex-col items-center justify-center min-h-[60vh]">
        <Loader2 className="animate-spin text-brand mb-4" size={48} />
        <p className="text-text-secondary">Loading downloads...</p>
      </div>
    );
  }

  if (hasError) {
    return (
      <div className="p-8 pb-32 flex flex-col items-center justify-center min-h-[60vh]">
        <div className="mb-6 opacity-20">
          <XCircle size={80} className="text-red-400" />
        </div>
        <h1 className="text-2xl font-bold mb-2">Unable to Load Downloads</h1>
        <p className="text-[#6f7480] text-center max-w-sm">
          Could not connect to the backend. Make sure the server is running.
        </p>
      </div>
    );
  }

  if (downloads.length === 0) {
    return (
      <div className="p-8 pb-32 flex flex-col items-center justify-center min-h-[60vh]">
        <div className="mb-6 opacity-20">
          <Download size={80} />
        </div>
        <h1 className="text-2xl font-bold mb-2">No Downloads Yet</h1>
        <p className="text-[#6f7480] text-center max-w-sm">
          Download songs, albums, or playlists from Spotify to listen offline. Look for the download button on any track.
        </p>
      </div>
    );
  }

  return (
    <div className="p-8 pb-32">
      <div className="mb-8 flex justify-between items-end">
        <div>
          <h1 className="text-3xl font-bold mb-2 flex items-center gap-3">
            <Download className="text-brand" size={32} />
            Downloads
          </h1>
          <p className="text-text-secondary">
            Manage your Spotify downloads
          </p>
        </div>
        <div className="flex gap-2">
          {stats.failed > 0 && (
            <button
              onClick={handleRetryAllFailed}
              className="px-4 py-2 bg-surface-2 hover:bg-surface-3 text-text-secondary hover:text-brand rounded-lg transition-colors flex items-center gap-2"
            >
              <RotateCcw size={18} />
              Retry All Failed
            </button>
          )}
          {stats.completed > 0 && (
            <button
              onClick={handleClearCompleted}
              className="px-4 py-2 bg-surface-2 hover:bg-surface-3 text-text-secondary hover:text-green-400 rounded-lg transition-colors flex items-center gap-2"
            >
              <CheckCircle size={18} />
              Clear Completed
            </button>
          )}
          {downloads.some(d => d.status === 'queued') && (
            <button
              onClick={handleClearQueue}
              className="px-4 py-2 bg-surface-2 hover:bg-surface-3 text-text-secondary hover:text-red-400 rounded-lg transition-colors flex items-center gap-2"
            >
              <Trash2 size={18} />
              Clear Queue
            </button>
          )}
        </div>
      </div>

      {/* Stats */}
      <div className="grid grid-cols-3 gap-4 mb-6">
        <div className="bg-surface-1 p-4 rounded-lg">
          <div className="flex items-center gap-2 text-brand mb-1">
            <Loader2 size={18} className="animate-spin" />
            <span className="text-sm font-semibold">Active</span>
          </div>
          <p className="text-2xl font-bold">{stats.active}</p>
        </div>
        <div className="bg-surface-1 p-4 rounded-lg">
          <div className="flex items-center gap-2 text-green-400 mb-1">
            <CheckCircle size={18} />
            <span className="text-sm font-semibold">Completed</span>
          </div>
          <p className="text-2xl font-bold">{stats.completed}</p>
        </div>
        <div className="bg-surface-1 p-4 rounded-lg">
          <div className="flex items-center gap-2 text-red-400 mb-1">
            <XCircle size={18} />
            <span className="text-sm font-semibold">Failed</span>
          </div>
          <p className="text-2xl font-bold">{stats.failed}</p>
        </div>
      </div>

      {/* Filter Tabs */}
      <div className="flex gap-2 mb-6 border-b border-surface-border">
        <button
          onClick={() => setFilter('all')}
          className={`px-4 py-2 font-semibold transition-all border-b-2 ${
            filter === 'all'
              ? 'border-brand text-brand'
              : 'border-transparent text-text-secondary hover:text-text-main'
          }`}
        >
          All ({downloads.length})
        </button>
        <button
          onClick={() => setFilter('active')}
          className={`px-4 py-2 font-semibold transition-all border-b-2 ${
            filter === 'active'
              ? 'border-brand text-brand'
              : 'border-transparent text-text-secondary hover:text-text-main'
          }`}
        >
          Active ({stats.active})
        </button>
        <button
          onClick={() => setFilter('completed')}
          className={`px-4 py-2 font-semibold transition-all border-b-2 ${
            filter === 'completed'
              ? 'border-brand text-brand'
              : 'border-transparent text-text-secondary hover:text-text-main'
          }`}
        >
          Completed ({stats.completed})
        </button>
        <button
          onClick={() => setFilter('failed')}
          className={`px-4 py-2 font-semibold transition-all border-b-2 ${
            filter === 'failed'
              ? 'border-brand text-brand'
              : 'border-transparent text-text-secondary hover:text-text-main'
          }`}
        >
          Failed ({stats.failed})
        </button>
      </div>

      {/* Downloads List */}
      <div className="space-y-3">
        {filteredDownloads.map(download => (
          <DownloadCard key={download.id} download={download} onDelete={handleDelete} onRetry={handleRetry} />
        ))}
      </div>

      {/* Clear Queue Button - Shown only if there are queued downloads */}
      {downloads.some(d => d.status === 'queued') && (
        <div className="mt-6">
          <button
            onClick={handleClearQueue}
            className="w-full bg-red-500 hover:bg-red-600 text-white font-semibold py-2 rounded-lg transition-colors flex items-center justify-center gap-2"
          >
            <Trash2 size={18} />
            Clear Queue
          </button>
        </div>
      )}
    </div>
  );
};

interface DownloadCardProps {
  download: ApiSpotifyDownload;
  onDelete: (id: string) => void;
  onRetry: (id: string) => void;
}

const DownloadCard: React.FC<DownloadCardProps> = ({ download, onDelete, onRetry }) => {
  const getStatusIcon = () => {
    switch (download.status) {
      case 'queued':
        return <Clock className="text-text-muted" size={20} />;
      case 'downloading':
        return <Loader2 className="text-brand animate-spin" size={20} />;
      case 'completed':
        return <CheckCircle className="text-green-400" size={20} />;
      case 'failed':
        return <XCircle className="text-red-400" size={20} />;
    }
  };

  const getStatusText = () => {
    switch (download.status) {
      case 'queued':
        return 'Queued';
      case 'downloading':
        return `Downloading... ${download.progress.toFixed(0)}%`;
      case 'completed':
        return 'Completed';
      case 'failed':
        return 'Failed';
    }
  };

  const formatDate = (timestamp: number) => {
    const date = new Date(timestamp * 1000);
    const now = new Date();
    const diffMs = now.getTime() - date.getTime();
    const diffMins = Math.floor(diffMs / 60000);
    
    if (diffMins < 1) return 'Just now';
    if (diffMins < 60) return `${diffMins}m ago`;
    const diffHours = Math.floor(diffMins / 60);
    if (diffHours < 24) return `${diffHours}h ago`;
    const diffDays = Math.floor(diffHours / 24);
    if (diffDays < 7) return `${diffDays}d ago`;
    return date.toLocaleDateString();
  };

  return (
    <div className="bg-surface-1 hover:bg-surface-2 p-4 rounded-lg transition-colors">
      <div className="flex items-start gap-4">
        {/* Icon */}
        <div className="w-12 h-12 bg-surface-3 rounded flex items-center justify-center flex-shrink-0">
          <Music className="text-text-muted" size={20} />
        </div>

        {/* Info */}
        <div className="flex-1 min-w-0">
          <h3 className="font-semibold text-white truncate mb-1">{download.title}</h3>
          <p className="text-sm text-text-secondary truncate mb-1">{download.artist}</p>
          {download.album && (
            <p className="text-xs text-text-muted truncate mb-2">{download.album}</p>
          )}
          
          {/* Status */}
          <div className="flex items-center gap-2 mb-2">
            {getStatusIcon()}
            <span className="text-sm font-medium">{getStatusText()}</span>
            <span className="text-xs text-text-muted">• {formatDate(download.addedAt)}</span>
          </div>

          {/* Progress Bar */}
          {download.status === 'downloading' && (
            <div className="h-2 bg-surface-3 rounded-full overflow-hidden">
              <div 
                className="h-full bg-brand transition-all duration-300"
                style={{ width: `${download.progress}%` }}
              />
            </div>
          )}

          {/* Error Message */}
          {download.status === 'failed' && download.errorMessage && (
            <div className="bg-red-400/10 border border-red-400/30 rounded p-2 mt-2">
              <p className="text-xs text-red-400">{download.errorMessage}</p>
            </div>
          )}

          {/* File Path */}
          {download.status === 'completed' && download.filePath && (
            <p className="text-xs text-text-muted mt-2 font-mono truncate">
              {download.filePath}
            </p>
          )}
        </div>

        {/* Action Buttons */}
        <div className="flex items-center gap-1">
          {/* Retry Button - only for failed downloads */}
          {download.status === 'failed' && (
            <button
              onClick={() => onRetry(download.id)}
              className="p-2 hover:bg-surface-3 rounded-lg transition-colors group"
              title="Retry download"
            >
              <RotateCcw className="text-text-muted group-hover:text-brand" size={18} />
            </button>
          )}
          
          {/* Delete Button */}
          <button
            onClick={() => onDelete(download.id)}
            className="p-2 hover:bg-surface-3 rounded-lg transition-colors group"
            title={download.status === 'downloading' ? "Cancel download" : "Remove from list"}
          >
            <Trash2 className={`text-text-muted group-hover:text-red-400 ${download.status === 'downloading' ? 'text-red-400' : ''}`} size={18} />
          </button>
        </div>
      </div>
    </div>
  );
};
