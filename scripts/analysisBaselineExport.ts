import { detectBPM, type BPMResult } from '../lib/bpmDetection';
import { detectKey, type KeyDetectionResult } from '../lib/keyDetection';

type ManifestTrack = {
  id: string;
  path: string;
};

type BenchmarkManifest = {
  version: string;
  tracks: ManifestTrack[];
};

export type BrowserBaselineResult = {
  id: string;
  bpm?: number;
  key?: string;
  confidence?: number;
  tempoConfidence?: number;
  keyConfidence?: number;
};

export type BrowserBaselineResultSet = {
  algorithm: 'browser-baseline-v1';
  results: BrowserBaselineResult[];
};

const basename = (path: string): string => path.replace(/\\/g, '/').split('/').pop() ?? '';

export const indexManifestTracksByFilename = (manifest: BenchmarkManifest): Map<string, ManifestTrack> => {
  if (!manifest || typeof manifest.version !== 'string' || !Array.isArray(manifest.tracks)) {
    throw new Error('Manifest must contain a version and tracks array.');
  }
  const indexed = new Map<string, ManifestTrack>();
  for (const track of manifest.tracks) {
    const filename = basename(track.path).toLocaleLowerCase();
    if (!track.id || !filename) {
      throw new Error('Every manifest track requires an id and path.');
    }
    if (indexed.has(filename)) {
      throw new Error(`Manifest paths share the filename ${basename(track.path)}; select uniquely named files.`);
    }
    indexed.set(filename, track);
  }
  return indexed;
};

export const browserBaselineResult = (
  id: string,
  bpm: BPMResult | undefined,
  key: KeyDetectionResult | undefined,
): BrowserBaselineResult => {
  const result: BrowserBaselineResult = { id };

  // detectBPM historically returns 120/confidence 0 when it has insufficient
  // evidence. Omit it so analysisbench counts an unknown rather than a false
  // confident result.
  if (bpm && Number.isFinite(bpm.bpm) && bpm.bpm > 0 && Number.isFinite(bpm.confidence) && bpm.confidence > 0) {
    result.bpm = bpm.bpm;
    result.confidence = Math.min(1, bpm.confidence);
    result.tempoConfidence = Math.min(1, bpm.confidence);
  }
  if (key && key.keyFull && Number.isFinite(key.confidence) && key.confidence > 0) {
    result.key = key.keyFull;
    result.keyConfidence = Math.min(1, key.confidence);
  }
  return result;
};

const bindExporter = (): void => {
  const manifestInput = document.querySelector<HTMLInputElement>('#manifest');
  const audioInput = document.querySelector<HTMLInputElement>('#audio-files');
  const exportButton = document.querySelector<HTMLButtonElement>('#export');
  const status = document.querySelector<HTMLElement>('#status');
  if (!manifestInput || !audioInput || !exportButton || !status) {
    throw new Error('Baseline exporter controls are missing.');
  }

  let manifestTracks = new Map<string, ManifestTrack>();
  const setStatus = (message: string): void => { status.textContent = message; };

  manifestInput.addEventListener('change', async () => {
    const [file] = manifestInput.files ?? [];
    if (!file) return;
    try {
      manifestTracks = indexManifestTracksByFilename(JSON.parse(await file.text()) as BenchmarkManifest);
      audioInput.disabled = false;
      exportButton.disabled = false;
      setStatus(`Loaded ${manifestTracks.size} manifest tracks. Select matching audio files.`);
    } catch (error) {
      manifestTracks = new Map();
      audioInput.disabled = true;
      exportButton.disabled = true;
      setStatus(`Manifest error: ${error instanceof Error ? error.message : String(error)}`);
    }
  });

  exportButton.addEventListener('click', async () => {
    const files = [...(audioInput.files ?? [])];
    if (files.length === 0) {
      setStatus('Select one or more audio files.');
      return;
    }
    const unmatched = files.filter(file => !manifestTracks.has(file.name.toLocaleLowerCase()));
    if (unmatched.length > 0) {
      setStatus(`No manifest entry for: ${unmatched.map(file => file.name).join(', ')}`);
      return;
    }

    exportButton.disabled = true;
    const results: BrowserBaselineResult[] = [];
    try {
      for (const [index, file] of files.entries()) {
        const track = manifestTracks.get(file.name.toLocaleLowerCase());
        if (!track) continue;
        setStatus(`Analyzing ${index + 1}/${files.length}: ${file.name}`);
        const objectURL = URL.createObjectURL(file);
        try {
          const [bpm, key] = await Promise.all([
            detectBPM(objectURL).catch(() => undefined),
            detectKey(objectURL, { duration: 30 }).catch(() => undefined),
          ]);
          results.push(browserBaselineResult(track.id, bpm, key));
        } finally {
          URL.revokeObjectURL(objectURL);
        }
      }
      const resultSet: BrowserBaselineResultSet = { algorithm: 'browser-baseline-v1', results };
      const blob = new Blob([`${JSON.stringify(resultSet, null, 2)}\n`], { type: 'application/json' });
      const url = URL.createObjectURL(blob);
      const download = document.createElement('a');
      download.href = url;
      download.download = 'browser-baseline-results.json';
      download.click();
      URL.revokeObjectURL(url);
      setStatus(`Exported ${results.length} results. Compare with analysisbench using the same manifest.`);
    } finally {
      exportButton.disabled = false;
    }
  });
};

if (typeof document !== 'undefined') {
  bindExporter();
}
