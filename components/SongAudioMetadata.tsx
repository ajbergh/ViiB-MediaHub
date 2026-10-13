import { reportAudioReadFailure, withTransientReadRetry } from '../services/audioReadDiagnostics';
import { SpotifyCatalogEvidence } from './SpotifyCatalogEvidence';
import { useAudioMetadataRevision } from '../hooks/useAudioMetadataRevision';
import { ManualAudioMetadata } from './ManualAudioMetadata';
import { ImportedAnalysisPreview } from './ImportedAnalysisPreview';
import { ProviderResourceActions } from './ProviderResourceActions';
import { TrackThreeBandPreview } from './TrackThreeBandPreview';
import { useEffect, useState } from 'react';
import { api } from '../services/api';
import type { TrackAnalysisFeature, SpotifyScalarField } from '../services/trackAnalysisContracts';
import { useStore } from '../store';

const label = (key: string) => key.replace(/^spotify_/, '').replace(/_score$/, '').replaceAll('_', ' ');
const providerFieldLabel = (key: string) => ({
  tempo_bpm: 'Spotify tempo', key_mode: 'Spotify key / mode', provider_loudness_db: 'Spotify loudness',
  time_signature: 'Time signature', duration_seconds: 'Duration', duration_milliseconds: 'Duration',
  spotify_energy_score: 'Spotify energy', spotify_danceability_score: 'Spotify danceability',
  spotify_acousticness_score: 'Spotify acousticness', spotify_instrumentalness_score: 'Spotify instrumentalness',
  spotify_liveness_score: 'Spotify liveness', spotify_speechiness_score: 'Spotify speechiness',
  spotify_valence_score: 'Spotify valence',
} as Record<string, string>)[key] ?? label(key);
const value = (field: SpotifyScalarField) => typeof field.value === 'number'
  ? String(field.value)
  : `${['C', 'C♯', 'D', 'D♯', 'E', 'F', 'F♯', 'G', 'G♯', 'A', 'A♯', 'B'][field.value.tonic] ?? `Tonic ${field.value.tonic}`} ${field.value.mode === 1 ? 'major' : field.value.mode === 0 ? 'minor' : `mode ${field.value.mode}`}`;

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
  const { revision: metadataRevision, generation: metadataGeneration } = useAudioMetadataRevision(songId);
  const [refreshing, setRefreshing] = useState(false);
  const [snapshot, setSnapshot] = useState<{ songId: string; session: number; data?: TrackAnalysisFeature; error?: boolean }>();
  useEffect(() => {
    let active = true;
    const requestGeneration = metadataGeneration.current;
    setRefreshing(true);
    void withTransientReadRetry(() => api.getTrackAnalysisFeature(songId), () => active && requestGeneration === metadataGeneration.current).then(data => {
      if (active && requestGeneration === metadataGeneration.current) { setSnapshot({ songId, session, data }); setRefreshing(false); }
    }, error => { if (active && requestGeneration === metadataGeneration.current) { reportAudioReadFailure('audio_metadata', error); setSnapshot(previous => ({ songId, session, data: previous?.songId === songId && previous.session === session ? previous.data : undefined, error: true })); setRefreshing(false); } });
    return () => { active = false; };
  }, [songId, session, revision, metadataRevision, metadataGeneration]);
  const current = snapshot?.songId === songId && snapshot.session === session ? snapshot : undefined;
  const catalog = <SpotifyCatalogEvidence key="catalog" songId={songId} fingerprint={current?.data?.sourceFingerprint} />;
  if (!current) return <><p role="status">Loading audio metadata…</p>{catalog}</>;
  if (!current.data) return <><div><p role="status">Audio metadata could not be loaded.</p><button type="button" onClick={() => setRevision(value => value + 1)}>Retry audio metadata</button></div>{catalog}</>;
  const data = current.data;
  const provider = data.providerScalars;
  // Reset uncontrolled editor inputs when an authoritative save/reset result
  // changes the selected manual values while retaining the surrounding panel.
  const editorRevision = JSON.stringify(data.effectiveFields?.filter(field => field.key === 'time_signature' || field.key === 'local_energy_level').map(field => field.selected));
  const providerValue = (field: SpotifyScalarField) => `${field.durableImport ? 'Downloaded' : 'Spotify'} ${providerFieldLabel(field.key)}: ${value(field)} ${field.units.replaceAll('_', ' ')}${field.stale ? ' · stale' : ''}`;
  const providerFields = provider?.selected ?? [];
  const providerAttempts = provider?.attempts ?? [];
  const attemptValue = (attempt: NonNullable<TrackAnalysisFeature['providerScalars']>['attempts'][number]) => {
    const endpoint = attempt.endpoint === 'audio_features' ? 'Audio features' : 'Detailed analysis';
    const origin = attempt.durableImport ? 'Downloaded import · ' : 'Spotify · ';
    const state = attempt.state === 'not_returned' ? 'not returned' : attempt.state === 'invalid_field' ? `rejected${attempt.reason ? ` (${attempt.reason})` : ''}` : 'returned';
    return `${origin}${endpoint}: ${state} · checked ${new Date(attempt.checkedAt).toLocaleString()}${attempt.durableImport ? ' · retained with downloaded file' : ''}`;
  };
  const providerFieldEvidence = (keys: string[]) => {
    const matches = providerFields.filter(field => keys.includes(field.key));
    const latestAttempts = new Map<string, NonNullable<TrackAnalysisFeature['providerScalars']>['attempts'][number]>();
    for (const attempt of providerAttempts) {
      if (!keys.includes(attempt.key)) continue;
      const previous = latestAttempts.get(attempt.endpoint);
      if (!previous || Date.parse(attempt.checkedAt) > Date.parse(previous.checkedAt)) latestAttempts.set(attempt.endpoint, attempt);
    }
    const evidence = matches.map(providerValue);
    evidence.push(...Array.from(latestAttempts.values()).map(attemptValue));
    return evidence.length ? evidence.join('; ') : 'No eligible retained provider observation';
  };
  const measuredKey = data.measuredKeyTonic == null ? 'Unknown' : `${['C', 'C♯', 'D', 'D♯', 'E', 'F', 'F♯', 'G', 'G♯', 'A', 'A♯', 'B'][data.measuredKeyTonic]} ${data.measuredKeyMode ?? ''}`;
  const capabilityRows = [
    { capability: 'Tempo / BPM', local: `Effective: ${data.bpm ?? 'Unknown'} BPM (${data.bpmSource}); measured alternative: ${data.measuredBpm ?? 'Unknown'} BPM${data.measuredBpmConfidence == null ? '' : ` · ${Math.round(data.measuredBpmConfidence * 100)}% confidence`}`, provider: providerFieldEvidence(['tempo_bpm']) },
    { capability: 'Key / mode', local: `Effective: ${data.key ?? 'Unknown'} (${data.keySource}); measured alternative: ${measuredKey}${data.measuredKeyConfidence == null ? '' : ` · ${Math.round(data.measuredKeyConfidence * 100)}% confidence`}`, provider: providerFieldEvidence(['key_mode']) },
    { capability: 'DJ energy', local: `Local scale: ${data.measuredEnergyLevel ?? (data.energyLevelSource === 'manual' ? undefined : data.energyLevel) ?? 'Unknown'} / 10${data.energyLevelConfidence == null ? '' : ` · ${Math.round(data.energyLevelConfidence * 100)}% confidence`}`, provider: providerFieldEvidence(['spotify_energy_score']) },
    { capability: 'Loudness / true peak', local: `BS.1770 integrated: ${data.integratedLufsBs1770 ?? 'Unknown'} LUFS · true peak: ${data.truePeakDbtp ?? 'Unknown'} dBTP`, provider: providerFieldEvidence(['provider_loudness_db']) },
    { capability: 'Recording duration', local: 'Playback duration follows the current local file.', provider: providerFieldEvidence(['duration_seconds', 'duration_milliseconds']) },
    { capability: 'Beat grid / phase', local: data.syncAllowed ? 'Tempo is sync-eligible; grid timing/provenance remains in the Grid tools. The default four-beat grouping is not measured meter.' : 'No current sync-eligible tempo; do not infer beat phase from Spotify tempo alone.', provider: 'Provider tempo does not supply local beat phase.' },
    { capability: 'Waveform bands', local: 'Current-source local amplitude / three-band estimates are shown below when prepared; they are not beat-alignment evidence.', provider: 'Native Spotify waveform is shown separately when retained.' },
    { capability: 'Sections / cue suggestions', local: data.structureAvailable ? 'Local energy-window sections and generated cue candidates are available; labels are not Spotify semantic sections or measured downbeats.' : 'No local section or cue analysis is currently available.', provider: 'Retained provider sections are inspected separately in Detailed arrays.' },
    { capability: 'Meter / time signature', local: 'Not estimated locally. A four-beat working grid is not measured meter.', provider: providerFieldEvidence(['time_signature']) },
    { capability: 'Bars / downbeats', local: 'Beat-grid downbeat indices are inferred from the working meter, not estimated meter or a measured bar-phase model.', provider: 'Retained provider bars and timing, when available, are inspected separately in Detailed arrays.' },
    { capability: 'Subdivisions / tatums', local: 'Not estimated locally; do not infer subdivisions by halving beat intervals.', provider: 'Retained provider timing, when available, is inspected separately in Detailed arrays.' },
    { capability: 'Segments / chroma / timbre', local: 'No local segment, chroma, or Spotify-compatible timbre-vector estimate. Local key is not an equivalent vector.', provider: 'Retained provider arrays are shown separately in Detailed arrays.' },
    { capability: 'Perceptual attributes', local: 'No local replacement for danceability, acousticness, instrumentalness, liveness, speechiness, or valence.', provider: providerFieldEvidence(['spotify_danceability_score', 'spotify_acousticness_score', 'spotify_instrumentalness_score', 'spotify_liveness_score', 'spotify_speechiness_score', 'spotify_valence_score']) },
  ];
  return <><section aria-label="Audio metadata" className="space-y-3 text-xs text-text-secondary">
    <h3 className="font-semibold text-text-main">Audio metadata</h3>
    {refreshing && <p role="status">Refreshing audio metadata… Showing the previous snapshot.</p>}
    {current.error && <p role="status">Audio metadata refresh failed. Showing the previous snapshot. <button type="button" onClick={() => setRevision(value => value + 1)}>Retry audio metadata</button></p>}
    <div className="rounded-lg border border-surface-border p-3">
      <p>BPM {data.bpm ?? 'Unknown'} · {data.bpmSource} · Key {data.key ?? 'Unknown'} · {data.keySource}</p>
      <p>Local DJ energy {data.measuredEnergyLevel ?? (data.energyLevelSource === 'manual' ? undefined : data.energyLevel) ?? 'Unknown'} / 10</p>
      <p>Local integrated loudness {data.integratedLufsBs1770 ?? 'Unknown'} LUFS</p>
      <details><summary>Local tempo and key alternatives</summary>
        <p>Measured BPM {data.measuredBpm ?? 'Unknown'}</p>
        <p>Measured key {data.measuredKeyTonic == null ? 'Unknown' : ['C', 'C♯', 'D', 'D♯', 'E', 'F', 'F♯', 'G', 'G♯', 'A', 'A♯', 'B'][data.measuredKeyTonic]} {data.measuredKeyMode ?? ''}</p>
      </details>
    </div>
    <ManualAudioMetadata key={`${songId}:${session}:${data.sourceFingerprint ?? 'unknown'}:${editorRevision}`} songId={songId} data={data} onReload={() => setRevision(value => value + 1)} />
    <details className="rounded-lg border border-surface-border p-3">
      <summary>Local analysis coverage and limits</summary>
      <p className="mt-2">Local DJ energy uses a 1-10 scale. It is separate from Spotify energy and other 0-1 scores.</p>
      <p className="mt-2">This inventory separates current local measurements, explicit local gaps and independently retained Spotify evidence. A provider observation is not a local measurement or a claim that an endpoint is available.</p>
      <div className="mt-2 overflow-x-auto" role="region" aria-label="Audio capability status table" tabIndex={0}>
        <table aria-label="Audio analysis capability status" className="w-full text-left">
          <thead><tr><th scope="col" className="pr-3">Capability</th><th scope="col" className="pr-3">Local status</th><th scope="col">Spotify status</th></tr></thead>
          <tbody>{capabilityRows.map(row => <tr key={row.capability}>
            <th scope="row" className="py-1 pr-3 align-top text-text-main">{row.capability}</th>
            <td className="py-1 pr-3 align-top">{row.local}</td>
            <td className="py-1 align-top">{row.provider}</td>
          </tr>)}</tbody>
        </table>
      </div>
      <p className="mt-2">A four-beat grid is a working default, not measured meter. Band overviews show spectral level estimates; they do not establish beat alignment or transition quality.</p>
      <p className="mt-2">Missing Spotify fields do not imply zero. Use current local tempo, key, energy and cue evidence where available, and audition transitions.</p>
    </details>
    {data.sourceFingerprint && <ImportedAnalysisPreview key={`${songId}:${data.sourceFingerprint}`} songId={songId} fingerprint={data.sourceFingerprint} />}
    <h4 className="font-semibold text-text-main">Spotify observations</h4>
    {data.sourceFingerprint && <TrackThreeBandPreview songId={songId} fingerprint={data.sourceFingerprint} recordingId={provider?.recordingId} session={session} />}
    {data.sourceFingerprint && <ProviderResourceActions songId={songId} fingerprint={data.sourceFingerprint} session={session} onReload={() => setRevision(value => value + 1)} />}
    {provider ? <>
      {provider.unverified && <p>Retained Spotify observations · Account confirmation pending. Available for viewing only.</p>}
      {provider.durableImportStatus && <p>Downloaded audio/scalar import: {provider.durableImportStatus.state === 'available' ? 'some provider evidence retained; scalar fields may be empty' : provider.durableImportStatus.state === 'oversized' ? 'too large to retain' : 'no eligible provider evidence at download time'} · checked {new Date(provider.durableImportStatus.checkedAt).toLocaleString()}</p>}
      {(provider.provenance === 'spotify_download_import' || provider.provenance === 'spotify_mixed_private_and_download_import') && <p>Downloaded Spotify observations · retained with the current file and available offline{provider.provenance === 'spotify_mixed_private_and_download_import' ? ' alongside current account cache' : ''}.</p>}
      <a href={`https://open.spotify.com/track/${provider.recordingId}`} target="_blank" rel="noopener noreferrer">Open Spotify recording</a>
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-2">{provider.selected.map(field => <ProviderField key={field.key} field={field} />)}</div>
      <details><summary>All provider observations</summary>{provider.fields.map((field, index) => <ProviderField key={`${field.key}:${field.endpoint}:${field.durableImport ? 'download' : 'private'}:${field.retrievedAt}:${index}`} field={field} />)}</details>
      <details><summary>Latest field availability</summary>{provider.attempts.map((attempt, index) => <p key={`${attempt.key}:${attempt.endpoint}:${attempt.durableImport ? 'download' : 'private'}:${attempt.checkedAt}:${index}`}>{label(attempt.key)} · {attempt.endpoint === 'audio_features' ? 'Audio features' : 'Detailed analysis'} · {attempt.state === 'available' ? 'Available' : attempt.state === 'not_returned' ? 'Not returned' : 'Invalid field'} · {new Date(attempt.checkedAt).toLocaleString()}</p>)}</details>
    </> : <p>No eligible Spotify observations for this file.</p>}
  </section>{catalog}</>;
}
