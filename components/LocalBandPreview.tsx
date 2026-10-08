import { useEffect, useState } from 'react';
import { bandDisplay, loadLocalThreeBand, type LocalThreeBand } from '../services/localThreeBand';

export function LocalBandPreview({ songId, fingerprint }: { songId: string; fingerprint: string }) {
  const [revision, setRevision] = useState(0);
  const [snapshot, setSnapshot] = useState<{ id: string; fp: string; data: LocalThreeBand | null; error?: boolean }>();
  useEffect(() => {
    let active = true;
    setSnapshot(undefined);
    void loadLocalThreeBand(songId, fingerprint).then(data => {
      if (active) setSnapshot({ id: songId, fp: fingerprint, data: data?.sourceFingerprint === fingerprint ? data : null });
    }, () => { if (active) setSnapshot({ id: songId, fp: fingerprint, data: null, error: true }); });
    return () => { active = false; };
  }, [songId, fingerprint, revision]);
  const current = snapshot?.id === songId && snapshot.fp === fingerprint ? snapshot : undefined;
  if (!current) return <p role="status">Loading local bands…</p>;
  if (!current.data) return <div><p>{current.error ? 'Local bands could not be loaded.' : 'Local bands are not prepared for the current file.'}</p><button type="button" onClick={() => setRevision(value => value + 1)}>Retry local bands</button></div>;
  const o = current.data.overview;
  const display = bandDisplay([o.low, o.mid, o.high]);
  return <section aria-label="Local three-band waveform" className="space-y-2">
    <h4 className="font-semibold text-text-main">Local three-band estimate</h4>
    {display.paths.map((path, i) => <div key={i}>
      <p>{['Low', 'Mid', 'High'][i]}</p>
      <svg role="img" aria-label={`${['Low', 'Mid', 'High'][i]} band envelope`} viewBox="0 0 600 75" className="h-16 w-full text-brand" preserveAspectRatio="none"><path d={path} fill="none" stroke="currentColor" strokeWidth="1.5" /></svg>
    </div>)}
    <p>{(o.frames / o.sampleRate).toFixed(2)} seconds · Common peak display scale{display.peak === 0 ? ' · Silence' : ''}</p>
    <details><summary>Measurement details</summary><p>Local filtered PCM peaks · 250 Hz / 4 kHz crossover estimates · Equal-channel mono</p><p>Window {(o.resolution / o.sampleRate * 1000).toFixed(2)} ms · Stored values have no normalization</p></details>
  </section>;
}
