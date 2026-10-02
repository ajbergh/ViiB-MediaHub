import React, { useState } from 'react';
import { ListMusic } from 'lucide-react';
import { generateGradient } from '../utils';

interface PlaylistArtworkProps {
  name: string;
  coverUrl?: string;
  covers: string[];
  className?: string;
}

/** A live track-art thumbnail; no generated blob URLs need to be persisted. */
export const PlaylistArtwork: React.FC<PlaylistArtworkProps> = ({ name, coverUrl, covers, className = '' }) => {
  const [failedUrls, setFailedUrls] = useState<Set<string>>(() => new Set());
  const available = covers.filter(url => !failedUrls.has(url));
  const customCover = coverUrl && !failedUrls.has(coverUrl) ? coverUrl : undefined;
  const tiles = customCover ? [customCover] : available.length > 1
    ? Array.from({ length: 4 }, (_, index) => available[index % available.length])
    : available;
  return (
    <div
      className={`w-full aspect-square rounded-md shadow-lg relative overflow-hidden ${className}`}
      style={{ background: generateGradient(name) }}
      aria-hidden="true"
    >
      {tiles.length ? (
        <div className={`absolute inset-0 grid ${tiles.length === 1 ? 'grid-cols-1 grid-rows-1' : 'grid-cols-2 grid-rows-2'}`}>
          {tiles.map((url, index) => (
            <img
              key={`${index}:${url}`}
              src={url}
              alt=""
              loading="lazy"
              className="w-full h-full min-h-0 min-w-0 object-cover"
              onError={() => setFailedUrls(previous => new Set([...previous, url]))}
            />
          ))}
        </div>
      ) : (
        <div className="absolute inset-0 flex items-center justify-center">
          <ListMusic size={40} className="text-white/70" />
        </div>
      )}
    </div>
  );
};
