import React, { useState } from 'react';
import { Link2, Loader2, X, Music, Disc3, ListMusic, CheckCircle, AlertCircle } from 'lucide-react';
import api from '../services/api';

interface DirectDownloadDialogProps {
  isOpen: boolean;
  onClose: () => void;
}

// Regex patterns for Spotify URL/URI validation
const SPOTIFY_URL_REGEX = /^https?:\/\/open\.spotify\.com\/(track|album|playlist)\/[a-zA-Z0-9]+/;
const SPOTIFY_URI_REGEX = /^spotify:(track|album|playlist):[a-zA-Z0-9]+$/;

const validateSpotifyInput = (input: string): { valid: boolean; type?: string } => {
  const trimmed = input.trim();
  
  if (SPOTIFY_URL_REGEX.test(trimmed)) {
    const match = trimmed.match(/\/(track|album|playlist)\//);
    return { valid: true, type: match ? match[1] : undefined };
  }
  
  if (SPOTIFY_URI_REGEX.test(trimmed)) {
    const match = trimmed.match(/spotify:(track|album|playlist):/);
    return { valid: true, type: match ? match[1] : undefined };
  }
  
  return { valid: false };
};

const getTypeIcon = (type?: string) => {
  switch (type) {
    case 'track':
      return <Music size={16} className="text-brand" />;
    case 'album':
      return <Disc3 size={16} className="text-brand" />;
    case 'playlist':
      return <ListMusic size={16} className="text-brand" />;
    default:
      return null;
  }
};

const getTypeName = (type?: string) => {
  switch (type) {
    case 'track':
      return 'Track';
    case 'album':
      return 'Album';
    case 'playlist':
      return 'Playlist';
    default:
      return '';
  }
};

export const DirectDownloadDialog: React.FC<DirectDownloadDialogProps> = ({ isOpen, onClose }) => {
  const [url, setUrl] = useState('');
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<{ type: string; title: string; count?: number } | null>(null);

  const validation = url.trim() ? validateSpotifyInput(url) : { valid: false };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    
    if (!validation.valid) {
      setError('Please enter a valid Spotify URL or URI');
      return;
    }

    setIsLoading(true);
    setError(null);
    setSuccess(null);

    try {
      const result = await api.downloadFromURL(url.trim());
      setSuccess({
        type: result.type,
        title: result.title,
        count: result.count,
      });
      setUrl('');
      
      // Auto-close after 2 seconds on success
      setTimeout(() => {
        onClose();
        setSuccess(null);
      }, 2000);
    } catch (err: any) {
      setError(err.message || 'Failed to queue download');
    } finally {
      setIsLoading(false);
    }
  };

  const handleClose = () => {
    setUrl('');
    setError(null);
    setSuccess(null);
    onClose();
  };

  if (!isOpen) return null;

  return (
    <div 
      className="fixed inset-0 z-[100] flex items-center justify-center bg-black/80 backdrop-blur-sm animate-in fade-in duration-200 motion-reduce:animate-none motion-reduce:transition-none"
      onClick={(e) => {
        if (e.target === e.currentTarget && !isLoading) {
          handleClose();
        }
      }}
    >
      <div className="bg-surface-2 border border-surface-border rounded-xl p-6 max-w-lg w-full mx-4 shadow-2xl">
        {/* Header */}
        <div className="flex items-center justify-between mb-4">
          <div className="flex items-center gap-3">
            <div className="p-2 bg-brand/20 rounded-lg">
              <Link2 size={20} className="text-brand" />
            </div>
            <div>
              <h2 className="text-lg font-bold text-text-main">Direct Download</h2>
              <p className="text-sm text-text-secondary">Paste a Spotify URL or URI</p>
            </div>
          </div>
          <button
            onClick={handleClose}
            disabled={isLoading}
            className="p-2 hover:bg-surface-3 rounded-lg transition-colors disabled:opacity-50"
          >
            <X size={20} />
          </button>
        </div>

        {/* Form */}
        <form onSubmit={handleSubmit}>
          <div className="mb-4">
            <div className="relative">
              <input
                type="text"
                value={url}
                onChange={(e) => {
                  setUrl(e.target.value);
                  setError(null);
                  setSuccess(null);
                }}
                placeholder="https://open.spotify.com/track/... or spotify:track:..."
                className={`w-full bg-surface-1 border rounded-lg px-4 py-3 pr-12 text-text-main placeholder-text-muted focus:outline-none focus:ring-2 focus:ring-brand/50 transition-all ${
                  error ? 'border-error' : validation.valid ? 'border-success' : 'border-surface-border'
                }`}
                disabled={isLoading}
                autoFocus
              />
              {url.trim() && (
                <div className="absolute right-3 top-1/2 -translate-y-1/2 flex items-center gap-2">
                  {validation.valid ? (
                    <>
                      {getTypeIcon(validation.type)}
                      <span className="text-xs text-text-secondary">{getTypeName(validation.type)}</span>
                    </>
                  ) : (
                    <AlertCircle size={16} className="text-error" />
                  )}
                </div>
              )}
            </div>
            
            {/* Validation hint */}
            {url.trim() && !validation.valid && (
              <p className="text-xs text-error mt-2">
                Invalid format. Use a Spotify URL or URI for a track, album, or playlist.
              </p>
            )}
          </div>

          {/* Error message */}
          {error && (
            <div className="mb-4 p-3 bg-error/10 border border-error/30 rounded-lg">
              <p className="text-sm text-error">{error}</p>
            </div>
          )}

          {/* Success message */}
          {success && (
            <div className="mb-4 p-3 bg-success/10 border border-success/30 rounded-lg flex items-center gap-3">
              <CheckCircle size={20} className="text-success flex-shrink-0" />
              <div>
                <p className="text-sm text-success font-medium">
                  {success.type === 'track' 
                    ? `Queued "${success.title}"`
                    : `Queued ${success.count} tracks from "${success.title}"`
                  }
                </p>
              </div>
            </div>
          )}

          {/* Supported formats hint */}
          <div className="mb-4 text-xs text-text-muted">
            <p className="mb-1">Supported formats:</p>
            <ul className="list-disc list-inside space-y-0.5 ml-1">
              <li>https://open.spotify.com/track/ID</li>
              <li>https://open.spotify.com/album/ID</li>
              <li>https://open.spotify.com/playlist/ID</li>
              <li>spotify:track:ID / spotify:album:ID / spotify:playlist:ID</li>
            </ul>
          </div>

          {/* Actions */}
          <div className="flex items-center justify-end gap-3">
            <button
              type="button"
              onClick={handleClose}
              disabled={isLoading}
              className="px-4 py-2 rounded-lg font-medium text-text-main hover:bg-surface-3 transition-colors disabled:opacity-50"
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={isLoading || !validation.valid}
              className="px-5 py-2 rounded-lg font-bold bg-brand hover:bg-brand/90 text-white transition-colors flex items-center gap-2 disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {isLoading && <Loader2 size={16} className="animate-spin" />}
              {isLoading ? 'Queueing...' : 'Download'}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
};

export default DirectDownloadDialog;
