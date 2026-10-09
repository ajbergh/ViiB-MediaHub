import { useStore } from '../store';
import { reportAudioReadFailure } from '../services/audioReadDiagnostics';
import { useEffect, useRef, useState } from 'react';
import { bandDisplay, loadLocalThreeBand, type LocalThreeBand } from '../services/localThreeBand';
import { loadProviderThreeBand, refreshProviderThreeBand, signedBandPaths, type ProviderThreeBand } from '../services/providerThreeBand';

type Snapshot =
  | { identity: string; source: 'provider'; data: ProviderThreeBand }
  | { identity: string; source: 'local'; data: LocalThreeBand }
  | { identity: string; source: 'missing'; error: boolean };

export function TrackThreeBandPreview({ songId, fingerprint, recordingId, session }: { songId: string; fingerprint: string; recordingId?: string; session: number }) {
  const connected = useStore(state => state.spotifyConnected);
  const identity = `${songId}:${fingerprint}:${recordingId ?? ''}:${session}`;
  const current = useRef(identity);
  current.current = identity;
  const [revision, setRevision] = useState(0);
  const [snapshot, setSnapshot] = useState<Snapshot>();
  const [refreshMessage, setRefreshMessage] = useState<{ identity: string; text: string }>();
  useEffect(() => {
    current.current = identity;
    let active = true;
    const isCurrent = () => active && current.current === identity;
    setSnapshot(undefined);
    void (async () => {
      let providerFailed = false;
      try {
        const provider = await loadProviderThreeBand(songId);
        if (!isCurrent()) return;
        if (provider?.sourceFingerprint === fingerprint && (!recordingId || provider.recordingId === recordingId)) {
          setSnapshot({ identity, source: 'provider', data: provider });
          return;
        }
      } catch (error) {
        providerFailed = true;
        reportAudioReadFailure('provider_bands', error);
      }
      try {
        const local = await loadLocalThreeBand(songId, fingerprint, isCurrent);
        if (!isCurrent()) return;
        if (local?.sourceFingerprint === fingerprint) setSnapshot({ identity, source: 'local', data: local });
        else setSnapshot({ identity, source: 'missing', error: providerFailed });
      } catch (error) {
        if (!isCurrent()) return;
        reportAudioReadFailure('local_bands', error);
        setSnapshot({ identity, source: 'missing', error: true });
      }
    })();
    return () => { active = false; if (current.current === identity) current.current = ''; };
  }, [identity, songId, fingerprint, recordingId, revision]);

  const refresh = async () => {
    if (!recordingId) return;
    const requested = identity;
    setRefreshMessage({ identity, text: 'Refreshing Spotify waveform…' });
    try {
      const result = await refreshProviderThreeBand(recordingId);
      if (current.current !== requested) return;
      setRefreshMessage({ identity, text: result.state === 'available' ? 'Waveform refreshed.' : result.state === 'cooldown' ? 'Spotify is limiting requests. Try again later.' : 'Spotify waveform is unavailable. Showing the best current-source waveform.' });
      if (result.state === 'available') setRevision(value => value + 1);
    } catch {
      if (current.current === requested) setRefreshMessage({ identity, text: 'Waveform refresh failed. Showing the best current-source waveform.' });
    }
  };
  const visible = snapshot?.identity === identity ? snapshot : undefined;
  if (!visible) return <p role="status">Loading three-band waveform…</p>;
  if (visible.source === 'missing') return <div><p>{visible.error ? 'Three-band waveform could not be loaded.' : 'No three-band waveform is prepared for the current file.'}</p><button type="button" onClick={() => setRevision(value => value + 1)}>Retry waveform</button></div>;

  const provider = visible.source === 'provider' ? visible.data as ProviderThreeBand : undefined;
  const local = visible.source === 'local' ? visible.data as LocalThreeBand : undefined;
  const paths = provider ? signedBandPaths([provider.lows, provider.mids, provider.highs])
    : bandDisplay([local!.overview.low, local!.overview.mid, local!.overview.high]).paths;
  const refreshStatus = refreshMessage?.identity === identity ? refreshMessage.text : '';
  return <section aria-label="Three-band waveform" className="space-y-2">
    <div className="flex items-center justify-between gap-2">
      <h4 className="font-semibold text-text-main">{provider ? `Spotify three-band waveform${provider.stale ? ' · Stale' : ''}` : 'Local three-band estimate'}</h4>
      {recordingId && <button type="button" className="rounded border border-surface-border px-3 py-1 disabled:opacity-50" disabled={!connected} onClick={() => void refresh()}>Refresh Spotify waveform</button>}
    </div>
    {provider?.provenance === 'spotify_durable_import' && <p>Retained with this downloaded file. Available independently of the connected Spotify account.</p>}
    {provider?.unverified && <p>Retained Spotify waveform · Account confirmation pending. Available for viewing only.</p>}
    {provider && <p>{provider.alignment === 'duration_compatible' ? 'Duration compatible with this file; beat alignment is not established.' : provider.alignment === 'duration_mismatch' ? 'Duration differs from this file. Shown on the Spotify recording timeline.' : 'Local decoded duration is unavailable. Shown on the Spotify recording timeline.'}</p>}
    {paths.map((path, i) => <div key={i}><p>{['Low', 'Mid', 'High'][i]}</p><svg role="img" aria-label={`${provider ? 'Spotify' : 'Local'} ${['low', 'mid', 'high'][i]} band`} viewBox={`0 0 600 ${provider ? 80 : 75}`} className="h-16 w-full text-brand" preserveAspectRatio="none"><path d={path} fill="none" stroke="currentColor" strokeWidth="1.5" /></svg></div>)}
    {provider ? <><p>Spotify duration {provider.durationSeconds.toFixed(2)} seconds · Common signed peak display scale</p><details><summary>Provider waveform details</summary><p>Native signed samples · {provider.windowMilliseconds} ms windows · Display peak aggregation</p><p>Retrieved {new Date(provider.retrievedAt).toLocaleString()}</p></details></> : <><p>{(local!.overview.frames / local!.overview.sampleRate).toFixed(2)} seconds · Common peak display scale</p><details><summary>Measurement details</summary><p>Local filtered PCM peaks · 250 Hz / 4 kHz crossover estimates · Equal-channel mono</p><p>Window {(local!.overview.resolution / local!.overview.sampleRate * 1000).toFixed(2)} ms · Stored values have no normalization</p></details></>}
    {refreshStatus && <p role="status">{refreshStatus}</p>}
    {recordingId && !connected && <p>Connect to Spotify in Settings to refresh.</p>}
  </section>;
}
