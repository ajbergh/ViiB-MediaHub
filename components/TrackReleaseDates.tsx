import React, { useEffect, useState } from 'react';
import { api, type TrackReleaseMetadata } from '../services/api';

const sourceLabel = (source?: string) => ({
  musicbrainz: 'MusicBrainz', spotify: 'Spotify', spotify_download_import: 'Spotify download',
  file_tag: 'File tags', manual: 'Manual', existing_metadata: 'Existing metadata', ai_inferred: 'AI estimate',
}[source || ''] || 'Unverified');

export const TrackReleaseDates: React.FC<{ songID: string }> = ({ songID }) => {
  const [metadata, setMetadata] = useState<TrackReleaseMetadata | null>(null);
  useEffect(() => {
    let active = true;
    setMetadata(null);
    void (async () => {
      try {
        const result = await api.getTrackReleaseMetadata(songID);
        if (active) setMetadata(result);
      } catch { /* Older/offline backends retain the existing year badges. */ }
    })();
    return () => { active = false; };
  }, [songID]);
  if (!metadata || (!metadata.originalDate && !metadata.editionDate)) return null;
  return (
    <dl className="mt-3 text-xs text-text-secondary space-y-1">
      {metadata.originalDate && <div><dt className="inline font-medium">Original release: </dt><dd className="inline">{metadata.originalDate} · {sourceLabel(metadata.originalSource)}</dd></div>}
      {metadata.editionDate && <div><dt className="inline font-medium">Edition release: </dt><dd className="inline">{metadata.editionDate} · {sourceLabel(metadata.editionSource)}</dd></div>}
    </dl>
  );
};
