import { useEffect, useRef, useState } from 'react';
import { spotifyReference, type SpotifyReferenceEndpoint } from '../services/spotifyReference';
import { useStore } from '../store';

export function ProviderResourceActions({ songId, fingerprint, session, onReload }: { songId: string; fingerprint: string; session: number; onReload: () => void }) {
  const connected = useStore(state => state.spotifyConnected);
  const identity = `${songId}:${fingerprint}:${session}`;
  const active = useRef(identity);
  active.current = identity;
  const [link, setLink] = useState<{ identity: string; recording: string | null }>();
  const [status, setStatus] = useState<{ identity: string; busy: boolean; message: string }>();
  useEffect(() => {
    active.current = identity;
    let current = true;
    setLink(undefined);
    void spotifyReference.link(songId).then(result => {
      if (current) setLink({ identity, recording: result.sourceFingerprint === fingerprint && result.link?.sourceFingerprint === fingerprint ? result.link.externalId : null });
    }, () => { if (current) setLink({ identity, recording: null }); });
    return () => { current = false; if (active.current === identity) active.current = ''; };
  }, [identity, songId, fingerprint]);
  const recording = link?.identity === identity ? link.recording : null;
  const visible = status?.identity === identity ? status : undefined;
  const refresh = async (endpoint: SpotifyReferenceEndpoint) => {
    if (!recording || !connected || visible?.busy) return;
    const requested = identity;
    setStatus({ identity, busy: true, message: 'Refreshing Spotify observations…' });
    try {
      const result = await spotifyReference.refresh(recording, endpoint);
      if (active.current !== requested) return;
      setStatus({ identity, busy: false, message: result.failure ? 'This Spotify resource is unavailable. Last-good observations are preserved.' : 'Spotify observations refreshed.' });
      if (!result.failure) onReload();
    } catch {
      if (active.current === requested) setStatus({ identity, busy: false, message: 'Refresh failed. Last-good observations are preserved.' });
    }
  };
  return <div className="space-y-2">
    <div className="flex flex-wrap gap-2">
      <button type="button" className="rounded border border-surface-border px-3 py-1 disabled:opacity-50" disabled={!connected || !recording || visible?.busy} onClick={() => void refresh('audio_features')}>Refresh audio features</button>
      <button type="button" className="rounded border border-surface-border px-3 py-1 disabled:opacity-50" disabled={!connected || !recording || visible?.busy} onClick={() => void refresh('audio_analysis')}>Refresh detailed analysis</button>
    </div>
    {!connected ? <p>Connect to Spotify in Settings to refresh.</p> : link && !recording ? <p>A current recording link is required to refresh.</p> : null}
    {visible?.message && <p role="status">{visible.message}</p>}
  </div>;
}
