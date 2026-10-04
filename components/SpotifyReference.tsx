import { useEffect, useRef, useState } from 'react';
import { useStore } from '../store';
import { spotifyReference, spotifyRecordingId, type SpotifyRecordingLink, type SpotifyReferenceView, type SpotifyReferenceEndpoint } from '../services/spotifyReference';

const failureText: Record<string, string> = {
  not_found: 'Spotify has no reference for this recording.',
  analysis_unavailable: 'Spotify has no usable reference for this recording.',
  authentication_required: 'Reconnect to Spotify to refresh this reference.',
  access_denied: 'Spotify denied access to this reference.',
  rate_limited: 'Spotify is limiting requests.',
  temporarily_unavailable: 'Spotify reference is temporarily unavailable.',
  provider_changed: 'Spotify reference could not be read.',
};

export function SpotifyReference({ songId, sourceIdentity }: { songId: string; sourceIdentity: string }) {
  const connected = useStore(state => state.spotifyConnected);
  const session = useStore(state => state.spotifySessionGeneration);
  const [identity, setIdentity] = useState<SpotifyRecordingLink | null>(null);
  const [view, setView] = useState<SpotifyReferenceView | null>(null);
  const [endpoint, setEndpoint] = useState<SpotifyReferenceEndpoint>('audio_features');
  const [input, setInput] = useState('');
  const [confirmed, setConfirmed] = useState(false);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState('');
  const generation = useRef(0);
  useEffect(() => {
    const current = ++generation.current;
    setIdentity(null); setView(null); setBusy(false); setMessage(''); setInput(''); setConfirmed(false);
    spotifyReference.link(songId).then(async value => {
      if (generation.current !== current) return;
      setIdentity(value);
      if (value.link) {
        const cached = await spotifyReference.cache(value.link.externalId, endpoint);
        if (generation.current === current) setView(cached);
      }
    }).catch(() => {
      if (generation.current === current) setMessage('Reference could not be loaded. Try again when the local library is available.');
    });
    return () => { generation.current++; };
  }, [songId, sourceIdentity, session, endpoint]);

  const run = async (action: () => Promise<void>) => {
    if (busy) return;
    const current = generation.current;
    setBusy(true); setMessage('');
    try { await action(); }
    catch (error) { if (current === generation.current) setMessage(error instanceof Error ? error.message : 'Reference could not be loaded.'); }
    finally { if (current === generation.current) setBusy(false); }
  };
  const link = () => run(async () => {
    const current = generation.current;
    const id = spotifyRecordingId(input);
    if (!id || !confirmed || !identity?.sourceFingerprint) throw new Error('Enter a Spotify track link and confirm the recording.');
    await spotifyReference.confirm(songId, id, identity.sourceFingerprint);
    const updated = await spotifyReference.link(songId);
    if (current !== generation.current) return;
    setIdentity(updated); setConfirmed(false);
    const cached = await spotifyReference.cache(id, endpoint);
    if (current === generation.current) setView(cached);
  });
  const refresh = () => run(async () => {
    const current = generation.current;
    if (!identity?.link || !connected) return;
    const id = identity.link.externalId;
    await spotifyReference.refresh(id, endpoint);
    const cached = await spotifyReference.cache(id, endpoint);
    if (current === generation.current) setView(cached);
  });
  const unlink = () => run(async () => {
    const current = generation.current;
    await spotifyReference.unlink(songId);
    if (current === generation.current) { setIdentity(previous => previous ? { ...previous, link: null } : null); setView(null); setInput(''); }
  });
  const observation = view?.cache?.observation;
  const failure = view?.lastFailure;
  return <section aria-label="Spotify reference" className="space-y-2 border-t border-[var(--dj-border)] pt-2 text-[var(--dj-text-secondary)]">
    <strong className="text-[var(--dj-text-primary)]">Spotify reference</strong>
    {identity && !identity.link && <>
      <label className="block">Spotify track link
        <input aria-label="Spotify recording link" value={input} onChange={event => setInput(event.target.value)} disabled={busy}
          className="ml-2 rounded border border-[var(--dj-border-light)] bg-[var(--dj-surface-1)] px-2 py-1" />
      </label>
      <label className="block"><input type="checkbox" checked={confirmed} onChange={event => setConfirmed(event.target.checked)} disabled={busy} /> This is the same recording and version as this local track.</label>
      <button type="button" onClick={() => void link()} disabled={busy || !confirmed || !identity.sourceFingerprint || !spotifyRecordingId(input)}>Link recording</button>
    </>}
    {identity?.link && <>
      <div className="flex flex-wrap gap-3">
        <a href={'https://open.spotify.com/track/' + identity.link.externalId} target="_blank" rel="noopener noreferrer">Open Spotify recording</a>
        <select className="rounded border border-[var(--dj-border-light)] bg-[var(--dj-surface-1)] px-1 text-[var(--dj-text-primary)]" aria-label="Spotify reference type" value={endpoint} disabled={busy} onChange={event => setEndpoint(event.target.value as SpotifyReferenceEndpoint)}>
          <option value="audio_features">Audio features</option><option value="audio_analysis">Detailed analysis</option>
        </select>
        <button type="button" disabled={busy || !connected} onClick={() => void refresh()}>Refresh reference</button>
        <button type="button" disabled={busy} onClick={() => void unlink()}>Remove recording link</button>
      </div>
      {observation ? <div>
        <p>BPM {observation.bpm ?? 'unknown'} · Key {observation.camelot ?? 'unknown'} · BPM confidence {observation.bpmConfidence == null ? 'unknown' : Math.round(observation.bpmConfidence * 100) + '%'} · Key confidence {observation.keyConfidence == null ? 'unknown' : Math.round(observation.keyConfidence * 100) + '%'}</p>
        <p>Spotify · {observation.sourceEndpoint === 'audio_features' ? 'Audio features' : 'Detailed analysis'} · Retrieved {new Date(observation.retrievedAt).toLocaleString()}{view?.stale ? ' · Stale' : ''}</p>
      </div> : <p>No cached reference.</p>}
      {failure && <p role="status">{failureText[failure.code] ?? 'Spotify reference is unavailable.'}{failure.code === 'rate_limited' ? ' Retry after ' + new Date(failure.retryAt).toLocaleString() + '.' : ''}</p>}
      {!connected && <p>Connect to Spotify in Settings to refresh.</p>}
    </>}
    {message && <p role="status">{message}</p>}
  </section>;
}
