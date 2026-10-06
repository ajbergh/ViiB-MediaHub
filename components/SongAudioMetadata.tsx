import { ImportedAnalysisPreview } from './ImportedAnalysisPreview';
import { ProviderResourceActions } from './ProviderResourceActions';
import { ProviderBandPreview } from './ProviderBandPreview';
import { LocalBandPreview } from './LocalBandPreview';
import { useEffect, useState } from 'react';
import { api } from '../services/api';
import type { TrackAnalysisFeature, SpotifyScalarField } from '../services/trackAnalysisContracts';
import { useStore } from '../store';

const label = (key: string) => key.replace(/^spotify_/, '').replace(/_score$/, '').replaceAll('_', ' ');
const value = (field: SpotifyScalarField) => typeof field.value === 'number'
  ? String(field.value)
  : `${['C', 'C♯', 'D', 'D♯', 'E', 'F', 'F♯', 'G', 'G♯', 'A', 'A♯', 'B'][field.value.tonic]} ${field.value.mode === 1 ? 'major' : 'minor'}`;

function ProviderField({ field }: { field: SpotifyScalarField }) {
  return <div className="rounded-lg border border-surface-border p-3">
    <div className="capitalize text-text-secondary">{label(field.key)}</div>
    <div className="font-mono text-text-main">{value(field)} <span className="text-text-subtle">{field.units.replaceAll('_', ' ')}</span>{field.stale && <span className="ml-2 text-text-secondary">Stale</span>}</div>
    {field.confidence != null && <div>Confidence {Math.round(field.confidence * 100)}%</div>}
    <details className="mt-1 text-text-subtle"><summary>Observation details</summary>
      <p>Spotify · {field.endpoint === 'audio_features' ? 'Audio features' : 'Detailed analysis'}</p>
      <p>Retrieved {new Date(field.retrievedAt).toLocaleString()}</p>
      <p>Fresh until {new Date(field.expiresAt).toLocaleString()}</p>
    </details>
  </div>;
}

export function SongAudioMetadata({ songId }: { songId: string }) {
  const session = useStore(state => state.spotifySessionGeneration);
  const [revision, setRevision] = useState(0);
  const [snapshot, setSnapshot] = useState<{ songId: string; session: number; data?: TrackAnalysisFeature; error?: boolean }>();
  useEffect(() => {
    let active = true;
    setSnapshot(undefined);
    void api.getTrackAnalysisFeature(songId).then(data => {
      if (active) setSnapshot({ songId, session, data });
    }, () => { if (active) setSnapshot({ songId, session, error: true }); });
    return () => { active = false; };
  }, [songId, session, revision]);
  const current = snapshot?.songId === songId && snapshot.session === session ? snapshot : undefined;
  if (!current) return <p role="status">Loading audio metadata…</p>;
  if (current.error || !current.data) return <p role="status">Audio metadata could not be loaded.</p>;
  const data = current.data;
  const provider = data.providerScalars;
  return <section aria-label="Audio metadata" className="space-y-3 text-xs text-text-secondary">
    <h3 className="font-semibold text-text-main">Audio metadata</h3>
    <div className="rounded-lg border border-surface-border p-3">
      <p>BPM {data.bpm ?? 'Unknown'} · {data.bpmSource} · Key {data.key ?? 'Unknown'} · {data.keySource}</p>
      <p>Local DJ energy {data.energyLevel ?? 'Unknown'} / 10</p>
      <p>Local integrated loudness {data.integratedLufsBs1770 ?? 'Unknown'} LUFS</p>
      <details><summary>Local tempo and key alternatives</summary>
        <p>Measured BPM {data.measuredBpm ?? 'Unknown'}</p>
        <p>Measured key {data.measuredKeyTonic == null ? 'Unknown' : ['C', 'C♯', 'D', 'D♯', 'E', 'F', 'F♯', 'G', 'G♯', 'A', 'A♯', 'B'][data.measuredKeyTonic]} {data.measuredKeyMode ?? ''}</p>
      </details>
    </div>
    <details className="rounded-lg border border-surface-border p-3">
      <summary>Local analysis coverage and limits</summary>
      <p className="mt-2">Local DJ energy uses a 1-10 scale. It is separate from Spotify energy and other 0-1 scores.</p>
      <p className="mt-2">Local estimates are not yet qualified for meter, downbeats, subdivisions, detailed segments, pitch/timbre vectors, danceability, acousticness, instrumentalness, liveness, speechiness or valence. Treat these fields as unavailable locally.</p>
      <p className="mt-2">A four-beat grid is a working default, not measured meter. Band overviews show spectral level estimates; they do not establish beat alignment or transition quality.</p>
      <p className="mt-2">Missing Spotify fields do not imply zero. Use current local tempo, key, energy and cue evidence where available, and audition transitions.</p>
    </details>
    {data.sourceFingerprint && <LocalBandPreview songId={songId} fingerprint={data.sourceFingerprint} />}
    {data.sourceFingerprint && <ImportedAnalysisPreview key={`${songId}:${data.sourceFingerprint}`} songId={songId} fingerprint={data.sourceFingerprint} />}
    <h4 className="font-semibold text-text-main">Spotify observations</h4>
    {data.sourceFingerprint && <ProviderBandPreview songId={songId} fingerprint={data.sourceFingerprint} recordingId={provider?.recordingId} session={session} />}
    {data.sourceFingerprint && <ProviderResourceActions songId={songId} fingerprint={data.sourceFingerprint} session={session} onReload={() => setRevision(value => value + 1)} />}
    {provider ? <>
      {provider.unverified && <p>Retained Spotify observations · Account confirmation pending. Available for viewing only.</p>}
      <a href={`https://open.spotify.com/track/${provider.recordingId}`} target="_blank" rel="noopener noreferrer">Open Spotify recording</a>
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-2">{provider.selected.map(field => <ProviderField key={field.key} field={field} />)}</div>
      <details><summary>All provider observations</summary>{provider.fields.map(field => <ProviderField key={`${field.key}:${field.endpoint}`} field={field} />)}</details>
      <details><summary>Latest field availability</summary>{provider.attempts.map(attempt => <p key={`${attempt.key}:${attempt.endpoint}`}>{label(attempt.key)} · {attempt.endpoint === 'audio_features' ? 'Audio features' : 'Detailed analysis'} · {attempt.state === 'available' ? 'Available' : attempt.state === 'not_returned' ? 'Not returned' : 'Invalid field'} · {new Date(attempt.checkedAt).toLocaleString()}</p>)}</details>
    </> : <p>No eligible Spotify observations for this file.</p>}
  </section>;
}
