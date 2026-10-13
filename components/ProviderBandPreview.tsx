import { useStore } from '../store';
import { useEffect, useRef, useState } from 'react';
import { loadProviderThreeBand, refreshProviderThreeBand, signedBandPaths, type ProviderThreeBand } from '../services/providerThreeBand';

function CachedProviderBandPreview({ songId, fingerprint, recordingId, session }: { songId: string; fingerprint: string; recordingId?: string; session: number }) {
  const identity = `${songId}:${fingerprint}:${recordingId}:${session}`;
  const [snapshot, setSnapshot] = useState<{ identity: string; data: ProviderThreeBand | null; error?: boolean }>();
  useEffect(() => {
    let active = true;
    setSnapshot(undefined);
    void loadProviderThreeBand(songId).then(data => {
      if (active) setSnapshot({ identity, data: data?.sourceFingerprint === fingerprint && (!recordingId || data.recordingId === recordingId) ? data : null });
    }, () => { if (active) setSnapshot({ identity, data: null, error: true }); });
    return () => { active = false; };
  }, [identity, songId, fingerprint, recordingId]);
  const current = snapshot?.identity === identity ? snapshot : undefined;
  if (!current) return <p role="status">Loading Spotify bands…</p>;
  if (!current.data) return <p>{current.error ? 'Spotify bands could not be loaded.' : 'No cached Spotify bands for this recording.'}</p>;
  const data = current.data;
  const paths = signedBandPaths([data.lows, data.mids, data.highs]);
  return <section aria-label="Spotify three-band waveform" className="space-y-2">
    <h4 className="font-semibold text-text-main">Spotify three-band waveform{data.stale ? ' · Stale' : ''}</h4>
    {data.provenance === 'spotify_durable_import' && <p>Retained with this downloaded file. Available independently of the connected Spotify account.</p>}
    {data.unverified && <p>Retained Spotify waveform · Account confirmation pending. Available for viewing only.</p>}
    <p>{data.alignment === 'duration_compatible' ? 'Duration compatible with this file; beat alignment is not established.' : data.alignment === 'duration_mismatch' ? 'Duration differs from this file. Shown on the Spotify recording timeline.' : 'Local decoded duration is unavailable. Shown on the Spotify recording timeline.'}</p>
    {paths.map((path, i) => <div key={i}><p>{['Low', 'Mid', 'High'][i]}</p><svg role="img" aria-label={`Spotify ${['low', 'mid', 'high'][i]} band`} viewBox="0 0 600 80" className="h-16 w-full text-brand" preserveAspectRatio="none"><path d={path} fill="none" stroke="currentColor" strokeWidth="1.5" /></svg></div>)}
    <p>Spotify duration {data.durationSeconds.toFixed(2)} seconds · Common signed peak display scale</p>
    <details><summary>Provider waveform details</summary><p>Native signed samples · {data.windowMilliseconds} ms windows · Display peak aggregation</p><p>Retrieved {new Date(data.retrievedAt).toLocaleString()}</p></details>
  </section>;
}


export function ProviderBandPreview(props: { songId: string; fingerprint: string; recordingId?: string; session: number }) {
  const connected = useStore(state => state.spotifyConnected);
  const identity = `${props.songId}:${props.fingerprint}:${props.recordingId}:${props.session}`;
  const current = useRef(identity);
  current.current = identity;
  const [revision, setRevision] = useState(0);
  const [status, setStatus] = useState<{ identity: string; busy: boolean; message: string }>();
  useEffect(() => { current.current = identity; return () => { if (current.current === identity) current.current = ''; }; }, [identity]);
  const visible = status?.identity === identity ? status : undefined;
  const refresh = async () => {
    if (!props.recordingId) return;
    const requested = identity;
    setStatus({ identity: requested, busy: true, message: 'Refreshing Spotify waveform…' });
    try {
      const result = await refreshProviderThreeBand(props.recordingId);
      if (current.current !== requested) return;
      setStatus({ identity: requested, busy: false, message: result.state === 'available' ? 'Waveform refreshed.' : result.state === 'cooldown' ? 'Spotify is limiting requests. Try again later.' : 'Spotify waveform is unavailable. Cached observations remain available.' });
      if (result.state === 'available') setRevision(value => value + 1);
    } catch {
      if (current.current === requested) setStatus({ identity: requested, busy: false, message: 'Waveform refresh failed. Cached observations remain available.' });
    }
  };
  return <div className="space-y-2">
    <CachedProviderBandPreview key={`${identity}:${revision}`} {...props} />
    <button type="button" className="rounded border border-surface-border px-3 py-1 disabled:opacity-50" disabled={!connected || !props.recordingId || visible?.busy} onClick={() => void refresh()}>Refresh Spotify waveform</button>
    {!connected && <p>Connect to Spotify in Settings to refresh.</p>}
    {visible?.message && <p role="status">{visible.message}</p>}
  </div>;
}
