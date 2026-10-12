// Read-only comparison of an already running local dev backend. No enrichment,
// scans, provider refreshes, recording changes or credential endpoints are used.
import { writeFile } from 'node:fs/promises';

const base = new URL(process.argv[2] || 'http://localhost:3000');
if (!['localhost', '127.0.0.1', '[::1]'].includes(base.hostname)) {
  throw new Error('This audit is restricted to a local dev server.');
}
async function get(path) {
  const start = performance.now();
  const response = await fetch(new URL(path, base), { signal: AbortSignal.timeout(20000) });
  if (!response.ok) throw new Error(`${path}: HTTP ${response.status}`);
  return { data: await response.json(), milliseconds: Math.round(performance.now() - start) };
}
const capturedAt = new Date().toISOString();
const [inventory, analysis, scan] = await Promise.all([
  get('/api/songs'), get('/api/v2/analysis'), get('/api/scan/status'),
]);
const rows = analysis.data;
const songs = new Map(inventory.data.map(song => [song.id, song]));
const counts = key => Object.fromEntries([...new Set(rows.map(row => row[key]))]
  .map(value => [value || 'absent', rows.filter(row => row[key] === value).length]));
const overlap = rows.filter(row => row.measuredBpm != null && row.providerScores);
const durationComparisons = rows.flatMap(row => {
  const selected = key => row.effectiveFields?.find(field => field.key === key)?.selected?.value;
  const local = selected('local_duration_seconds');
  const seconds = selected('duration_seconds');
  const milliseconds = selected('duration_milliseconds');
  const provider = typeof seconds === 'number' ? seconds
    : typeof milliseconds === 'number' ? milliseconds / 1000 : undefined;
  return typeof local === 'number' && typeof provider === 'number'
    ? [{ localSeconds: local, providerSeconds: provider, absoluteDifferenceSeconds: Math.abs(local - provider) }] : [];
});
const differences = durationComparisons.map(row => row.absoluteDifferenceSeconds).sort((a, b) => a - b);
const chosen = [...rows.filter(row => row.bpmSource === 'spotify').slice(0, 3),
  ...rows.filter(row => row.measuredBpm != null).slice(0, 3)];
const comparisons = [];
for (const row of chosen) {
  const { data: detail, milliseconds } = await get(`/api/v2/analysis/${encodeURIComponent(row.songId)}`);
  const song = songs.get(row.songId);
  const fields = Object.fromEntries((row.effectiveFields || []).filter(field => field.selected)
    .map(field => [field.key, { value: field.selected.value, units: field.selected.units,
      source: field.selected.source, stale: field.selected.stale }]));
  comparisons.push({ title: song?.title, artist: song?.artist,
    list: { bpm: row.bpm, bpmSource: row.bpmSource, key: row.key, keySource: row.keySource,
      measuredBpm: row.measuredBpm, measuredKeyTonic: row.measuredKeyTonic, measuredKeyMode: row.measuredKeyMode,
      syncAllowed: row.syncAllowed, structureAvailable: row.structureAvailable, fields },
    detail: { bpm: detail.bpm, bpmSource: detail.bpmSource, key: detail.key, keySource: detail.keySource,
      providerProvenance: detail.providerScalars?.provenance, syncAllowed: detail.syncAllowed,
      structureAvailable: detail.structureAvailable },
    detailListTimingSelectionAgrees: row.bpm === detail.bpm && row.bpmSource === detail.bpmSource
      && row.key === detail.key && row.keySource === detail.keySource,
	structureReadinessAgrees: row.structureAvailable === detail.structureAvailable,
    detailReadMilliseconds: milliseconds });
}
const report = { capturedAt, completedAt: new Date().toISOString(), base: base.origin,
  method: 'GET-only cache/local projection audit; no provider dispatch or library mutations',
  limitations: ['Point-in-time snapshot; background audio analysis may continue after file discovery.',
    'Running backend may predate uncommitted source changes; this is not a post-fix verification.',
    'Provider/local values are evidence, not independently reviewed accuracy labels.',
    'No browser, native rendering, offline/fresh-run or supported-platform qualification.'],
  librarySongs: inventory.data.length, analysisRows: rows.length, scan: scan.data,
  statuses: counts('status'), bpmSources: counts('bpmSource'), keySources: counts('keySource'),
  legacyAiTaggedSongs: inventory.data.filter(song => song.moodAnalyzedAt > 0).length,
  sameTrackMeasuredBpmAndProviderScores: overlap.length,
  durationComparison: { matchedRows: differences.length,
    maximumAbsoluteDifferenceSeconds: differences.at(-1),
    medianAbsoluteDifferenceSeconds: differences.length ? differences[Math.floor(differences.length / 2)] : undefined,
    rowsDifferingByMoreThanTwoSeconds: differences.filter(value => value > 2).length,
    interpretation: 'Decoded-file duration and provider recording duration remain separate. A difference is a review cue, not proof of a bad link. Local LUFS/energy and provider dB/0–1 energy are distinct metrics and are not numerically equated.' },
  sampledSelectionDisagreements: comparisons.filter(row => !row.detailListTimingSelectionAgrees).length,
  sampledStructureDisagreements: comparisons.filter(row => !row.structureReadinessAgrees).length,
  readMilliseconds: { inventory: inventory.milliseconds, analysis: analysis.milliseconds }, comparisons };
const json = JSON.stringify(report, null, 2) + '\n';
if (process.argv[3]) await writeFile(process.argv[3], json);
else process.stdout.write(json);
console.error(`Audited ${report.librarySongs} songs, ${rows.length} analysis rows; ${report.sampledSelectionDisagreements}/${comparisons.length} sampled detail/list disagreements.`);
