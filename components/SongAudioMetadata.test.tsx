// @vitest-environment jsdom
import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { beforeEach, expect, it, vi } from 'vitest';
const mocks = vi.hoisted(() => ({ load: vi.fn(), session: 0 }));
vi.mock('../services/api', () => ({ api: { getTrackAnalysisFeature: mocks.load } }));
vi.mock('../store', () => ({ useStore: (select: any) => select({ spotifySessionGeneration: mocks.session }) }));
import { SongAudioMetadata } from './SongAudioMetadata';
beforeEach(() => { mocks.load.mockReset(); });

it('keeps zero provider scores, stale provenance and local metrics separate', async () => {
  (globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;
  const field = { key: 'spotify_energy_score', metric: 'spotify_energy', units: 'unit_interval', value: 0, endpoint: 'audio_features', retrievedAt: '2026-10-01T00:00:00Z', expiresAt: '2026-10-02T00:00:00Z', stale: true };
  mocks.load.mockResolvedValue({ songId: 'a', bpmSource: 'unknown', keySource: 'unknown', energyLevel: 7, providerScalars: { unverified: true, readOnly: true, recordingId: 'recording', selected: [field], fields: [field], attempts: [] } });
  const host = document.createElement('div'); const root = createRoot(host);
  try {
    await act(async () => root.render(<SongAudioMetadata songId="a" />));
    expect(host.textContent).toContain('0 unit interval');
    expect(host.textContent).toContain('Account confirmation pending');
    expect(host.textContent).toContain('Available for viewing only');
    expect(host.textContent).toContain('Stale');
    expect(host.textContent).toContain('Local DJ energy 7 / 10');
    expect(host.textContent).toContain('Local integrated loudness Unknown LUFS');
    expect(host.textContent).toContain('Audio features');
    expect(host.textContent).toContain('This inventory separates current local measurements, explicit local gaps and independently retained Spotify evidence.');
    expect(host.querySelector('table[aria-label="Audio analysis capability status"]')).not.toBeNull();
    expect(host.querySelector('[role="region"][aria-label="Audio capability status table"]')?.getAttribute('tabindex')).toBe('0');
    expect(host.textContent).toContain('Meter / time signature');
    expect(host.textContent).toContain('Beat-grid downbeat indices are inferred from the working meter, not estimated meter or a measured bar-phase model.');
    expect(host.textContent).toContain('No local segment, chroma, or Spotify-compatible timbre-vector estimate.');
    expect(host.textContent).toContain('No local replacement for danceability, acousticness, instrumentalness, liveness, speechiness, or valence.');
    expect(host.textContent).toContain('No current sync-eligible tempo; do not infer beat phase from Spotify tempo alone.');
    expect(host.textContent).toContain('No local section or cue analysis is currently available.');
    expect(host.textContent).toContain('not measured meter');
    expect(host.textContent).toContain('Missing Spotify fields do not imply zero');
  } finally { await act(async () => root.unmount()); }
});

it('labels durable imported score provenance as available offline', async () => {
  (globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;
  const field = { key: 'spotify_energy_score', metric: 'spotify_energy', units: 'unit_interval', value: 0, endpoint: 'audio_features', schemaVersion: 1, adapterRevision: 'fixture', retrievedAt: '2026-10-01T00:00:00Z', expiresAt: '2026-10-02T00:00:00Z', stale: true, durableImport: true };
  mocks.load.mockResolvedValue({ songId: 'offline', bpmSource: 'unknown', keySource: 'unknown', providerScalars: { readOnly: true, provenance: 'spotify_download_import', recordingId: 'recording', sourceFingerprint: 'source-v1', selected: [field], fields: [field], attempts: [] } });
  const host = document.createElement('div'); const root = createRoot(host);
  try {
    await act(async () => root.render(<SongAudioMetadata songId="offline" />));
    expect(host.textContent).toContain('Downloaded Spotify observations · retained with the current file and available offline.');
    expect(host.textContent).toContain('Downloaded Spotify energy: 0 unit interval · stale');
  } finally { await act(async () => root.unmount()); }
});

it('labels retained provider meter separately from unavailable local meter estimation', async () => {
  (globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;
  const timeSignature = { key: 'time_signature', metric: 'measured_meter', units: 'beats_per_bar', value: 3, endpoint: 'audio_features', retrievedAt: '2026-10-01T00:00:00Z', expiresAt: '2026-10-02T00:00:00Z', stale: false };
  mocks.load.mockResolvedValue({ songId: 'meter', bpmSource: 'unknown', keySource: 'unknown', providerScalars: { recordingId: 'recording', selected: [timeSignature], fields: [timeSignature], attempts: [] } });
  const host = document.createElement('div'); const root = createRoot(host);
  try {
    await act(async () => root.render(<SongAudioMetadata songId="meter" />));
    const table = host.querySelector('table[aria-label="Audio analysis capability status"]');
    const meterRow = Array.from(table?.querySelectorAll('tbody tr') ?? []).find(row => row.textContent?.includes('Meter / time signature'));
    expect(meterRow).toBeDefined();
    expect(host.textContent).toContain('Meter / time signature');
    expect(host.textContent).toContain('Not estimated locally. A four-beat working grid is not measured meter.');
    expect(meterRow?.textContent).toContain('Time signature: 3 beats per bar');
    expect(host.textContent).not.toContain('Local measured meter 3');
  } finally { await act(async () => root.unmount()); }
});

it('shows a rejected provider attempt next to the retained last-good scalar', async () => {
  (globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;
  const retained = { key: 'tempo_bpm', metric: 'tempo', units: 'bpm', value: 120, endpoint: 'audio_features', retrievedAt: '2026-10-01T00:00:00Z', expiresAt: '2026-10-02T00:00:00Z', stale: true };
  const attempt = { key: 'tempo_bpm', endpoint: 'audio_analysis', state: 'invalid_field', reason: 'out_of_range', checkedAt: '2026-10-03T00:00:00Z', adapterRevision: 'fixture' };
  mocks.load.mockResolvedValue({ songId: 'attempts', bpm: 120, bpmSource: 'spotify', providerScalars: { recordingId: 'recording', fields: [retained], selected: [retained], attempts: [attempt] } });
  const host = document.createElement('div'); const root = createRoot(host);
  try {
    await act(async () => root.render(<SongAudioMetadata songId="attempts" />));
    const table = host.querySelector('table[aria-label="Audio analysis capability status"]');
    const tempoRow = Array.from(table?.querySelectorAll('tbody tr') ?? []).find(row => row.textContent?.includes('Tempo / BPM'));
    expect(tempoRow?.textContent).toContain('Spotify tempo: 120 bpm · stale');
    expect(tempoRow?.textContent).toContain('Spotify · Detailed analysis: rejected (out_of_range)');
  } finally { await act(async () => root.unmount()); }
});

it('distinguishes effective tempo and key, measured alternatives, and Spotify candidates', async () => {
  (globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;
  const tempo = { key: 'tempo_bpm', metric: 'tempo', units: 'bpm', value: 128, endpoint: 'audio_features', retrievedAt: '2026-10-01T00:00:00Z', expiresAt: '2026-10-02T00:00:00Z', stale: false };
  const key = { key: 'key_mode', metric: 'tonic_and_mode', units: 'pitch_class_and_mode', value: { tonic: 9, mode: 1 }, endpoint: 'audio_analysis', retrievedAt: '2026-10-01T00:00:00Z', expiresAt: '2026-10-02T00:00:00Z', stale: false };
  mocks.load.mockResolvedValue({ songId: 'effective', bpm: 120, bpmSource: 'measured', measuredBpm: 120, measuredBpmConfidence: .8, key: 'A minor', keySource: 'measured', measuredKeyTonic: 9, measuredKeyMode: 'minor', measuredKeyConfidence: .7, providerScalars: { recordingId: 'recording', selected: [tempo, key], fields: [tempo, key], attempts: [] } });
  const host = document.createElement('div'); const root = createRoot(host);
  try {
    await act(async () => root.render(<SongAudioMetadata songId="effective" />));
    const table = host.querySelector('table[aria-label="Audio analysis capability status"]');
    const rows = Array.from(table?.querySelectorAll('tbody tr') ?? []);
    const tempoRow = rows.find(row => row.textContent?.includes('Tempo / BPM'));
    const keyRow = rows.find(row => row.textContent?.includes('Key / mode'));
    expect(tempoRow?.textContent).toContain('Effective: 120 BPM (measured)');
    expect(tempoRow?.textContent).toContain('Spotify tempo: 128 bpm');
    expect(keyRow?.textContent).toContain('Effective: A minor (measured)');
    expect(keyRow?.textContent).toContain('Spotify key / mode: A major');
  } finally { await act(async () => root.unmount()); }
});

it('discards late results when the song changes', async () => {
  let resolve!: (value: unknown) => void;
  mocks.load.mockImplementationOnce(() => new Promise(done => { resolve = done; }));
  mocks.load.mockResolvedValue({ songId: 'b', bpm: 120, bpmSource: 'measured', keySource: 'unknown' });
  const host = document.createElement('div'); const root = createRoot(host);
  try {
    await act(async () => root.render(<SongAudioMetadata songId="a" />));
    await act(async () => root.render(<SongAudioMetadata songId="b" />));
    await act(async () => resolve({ songId: 'a', bpm: 199, bpmSource: 'spotify', keySource: 'unknown' }));
    expect(host.textContent).toContain('BPM 120');
    expect(host.textContent).not.toContain('199');
  } finally { await act(async () => root.unmount()); }
});

it('shows measured energy separately when the effective value is manual', async () => {
  (globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;
  mocks.load.mockResolvedValue({ songId: 'manual', bpmSource: 'unknown', keySource: 'unknown', energyLevel: 9, energyLevelSource: 'manual', measuredEnergyLevel: 4 });
  const host = document.createElement('div'); const root = createRoot(host);
  try {
    await act(async () => root.render(<SongAudioMetadata songId="manual" />));
    expect(host.textContent).toContain('Local DJ energy 4 / 10');
    expect(host.textContent).not.toContain('Local DJ energy 9 / 10');
  } finally { await act(async () => root.unmount()); }
});

it('reloads matching manual changes and rejects a pre-change response', async () => {
  (globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;
  let settle!: (value: unknown) => void;
  mocks.load.mockImplementationOnce(() => new Promise(done => { settle = done; }));
  mocks.load.mockResolvedValue({ songId: 'a', bpm: 120, bpmSource: 'measured', keySource: 'unknown' });
  const host = document.createElement('div'); const root = createRoot(host);
  const change = (songId: string) => window.dispatchEvent(new CustomEvent('library_updated', { detail: { source: 'manual_audio_metadata', songId, sourceFingerprint: 'fp', field: 'local_energy_level' } }));
  try {
    await act(async () => root.render(<SongAudioMetadata songId="a" />));
    await act(async () => change('other'));
    expect(mocks.load).toHaveBeenCalledTimes(1);
    await act(async () => { change('a'); settle({ songId: 'a', bpm: 199, bpmSource: 'spotify', keySource: 'unknown' }); });
    expect(mocks.load).toHaveBeenCalledTimes(2);
    expect(host.textContent).toContain('BPM 120');
    expect(host.textContent).not.toContain('199');
  } finally { await act(async () => root.unmount()); }
});

it('recovers an initial metadata failure through an explicit retry', async () => {
 (globalThis as any).IS_REACT_ACT_ENVIRONMENT=true;
 mocks.load.mockRejectedValueOnce(new Error('temporary')).mockResolvedValue({songId:'a',bpm:120,bpmSource:'measured',keySource:'unknown'});
 const host=document.createElement('div');const root=createRoot(host);
 try{
  await act(async()=>root.render(<SongAudioMetadata songId="a"/>));
  expect(host.textContent).toContain('could not be loaded');
  await act(async()=>(host.querySelector('button') as HTMLButtonElement).click());
  expect(host.textContent).toContain('BPM 120');
 }finally{await act(async()=>root.unmount());}
});

it('keeps the previous snapshot during refresh failure and hides it for another song', async () => {
 (globalThis as any).IS_REACT_ACT_ENVIRONMENT=true;
 let reject!: (error: Error)=>void;
 mocks.load.mockResolvedValueOnce({songId:'a',bpm:120,bpmSource:'measured',keySource:'unknown'}).mockImplementationOnce(()=>new Promise((_resolve,fail)=>{reject=fail;})).mockImplementation(()=>new Promise(()=>{}));
 const host=document.createElement('div');const root=createRoot(host);
 try{
  await act(async()=>root.render(<SongAudioMetadata songId="a"/>));
  await act(async()=>window.dispatchEvent(new CustomEvent('library_updated',{detail:{source:'manual_audio_metadata',songId:'a',sourceFingerprint:'fp',field:'local_energy_level'}})));
  expect(host.textContent).toContain('BPM 120');expect(host.textContent).toContain('Refreshing audio metadata');
  await act(async()=>reject(new Error('temporary')));
  expect(host.textContent).toContain('BPM 120');expect(host.textContent).toContain('refresh failed');
  await act(async()=>root.render(<SongAudioMetadata songId="b"/>));
  expect(host.textContent).not.toContain('BPM 120');
 }finally{await act(async()=>root.unmount());}
});
