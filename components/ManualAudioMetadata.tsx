import { notifyAudioMetadataChange } from '../hooks/useAudioMetadataRevision';
import { useEffect, useRef, useState } from 'react';
import { api } from '../services/api';
import type { TrackAnalysisFeature } from '../services/trackAnalysisContracts';

type Field = 'time_signature' | 'local_energy_level';
export function ManualAudioMetadata({ songId, data, onReload }: { songId: string; data: TrackAnalysisFeature; onReload: () => void }) {
  const alive = useRef(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  useEffect(() => { alive.current = true; return () => { alive.current = false; }; }, []);
  async function write(field: Field, input?: string) {
    const number = input === undefined ? undefined : Number(input);
    const max = field === 'time_signature' ? 32 : 10;
    if (number !== undefined && (!input?.trim() || !Number.isInteger(number) || number < 1 || number > max)) {
      setError(`Enter a whole number from 1 to ${max}.`); return;
    }
    if (!data.sourceFingerprint || busy) return;
    setBusy(true); setError('');
    try {
      if (number === undefined) await api.resetTrackMetadataField(songId, field, data.sourceFingerprint);
      else await api.updateTrackMetadataField(songId, field, number, data.sourceFingerprint);
      if (alive.current) notifyAudioMetadataChange({ source: 'manual_audio_metadata', songId, sourceFingerprint: data.sourceFingerprint, field });
    } catch {
      if (alive.current) { setError('The change could not be saved. Reload audio metadata before trying again.'); }
    } finally { if (alive.current) setBusy(false); }
  }
  return <fieldset disabled={busy || !data.sourceFingerprint} className="rounded-lg border border-surface-border p-3 space-y-2">
    <legend className="text-text-main">Manual audio metadata</legend>
    <p>Manual values stay with this file version. Reset restores the next eligible observation. Meter does not change beat-grid timing.</p>
    {([{ key: 'time_signature', name: 'Meter / time signature', max: 32, units: 'beats per bar' },
      { key: 'local_energy_level', name: 'Energy Level', max: 10, units: '/ 10' }] as const).map(field => {
      const selected = data.effectiveFields?.find(item => item.key === field.key)?.selected;
      return <div key={field.key}>
        <p>{field.name}: {typeof selected?.value === 'number' ? selected.value : 'Unknown'} {field.units} · {selected?.source ?? 'unknown'}</p>
        <form onSubmit={event => { event.preventDefault(); const input = new FormData(event.currentTarget).get('value'); void write(field.key, String(input ?? '')); }}>
          <label>Manual {field.name} <input name="value" type="number" min={1} max={field.max} step={1} required defaultValue={selected?.source === 'manual' && typeof selected.value === 'number' ? selected.value : ''} className="bg-surface-1 border border-surface-border rounded px-2 py-1 text-text-main" /></label>
          <button type="submit" className="ml-2">Save {field.name}</button>
          <button type="button" disabled={selected?.source !== 'manual'} onClick={() => void write(field.key)} className="ml-2">Reset {field.name}</button>
        </form>
      </div>;
    })}
    {!data.sourceFingerprint && <p>Current file identity is unavailable; editing is disabled.</p>}
    {busy && <p role="status">Saving audio metadata…</p>}
    {error && <p role="alert">{error} <button type="button" onClick={onReload}>Reload audio metadata</button></p>}
  </fieldset>;
}
