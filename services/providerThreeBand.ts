export interface ProviderThreeBand {
  songId: string; sourceFingerprint: string; recordingId: string;
  representation: 'spotify_three_band';
  lows: number[]; mids: number[]; highs: number[];
  sampleRate: number; windowMilliseconds: number; totalSamples: number; displayStride: number;
  displayAggregation: 'signed_absolute_max_v1';
  durationSeconds: number; localDurationSeconds: number;
  alignment: 'duration_compatible' | 'duration_mismatch' | 'local_duration_unavailable';
  retrievedAt: string; stale: boolean;
  readOnly?: boolean; unverified?: boolean; provenance?: 'spotify_private_cache' | 'spotify_durable_import';
}
export async function refreshProviderThreeBand(recordingId: string): Promise<{ state: string; retryAt?: string }> {
  if (!/^[A-Za-z0-9]{22}$/.test(recordingId)) throw new Error('Invalid recording');
  const response = await fetch(`/api/spotify/analysis/${encodeURIComponent(recordingId)}/waveform/refresh`, { method: 'POST', cache: 'no-store' });
  if (!response.ok) throw new Error('Waveform refresh unavailable');
  const result = await response.json() as { state: string; retryAt?: string };
  if (!['available', 'unavailable', 'failed', 'cooldown'].includes(result.state)) throw new Error('Invalid waveform status');
  return result;
}
export async function loadProviderThreeBand(songId: string): Promise<ProviderThreeBand | null> {
  const response = await fetch(`/api/v2/analysis/${encodeURIComponent(songId)}/waveform/spotify-three-band`, { cache: 'no-store' });
  if (response.status === 404) return null;
  if (!response.ok) throw new Error('Provider waveform unavailable');
  const data = await response.json() as ProviderThreeBand;
  if (data.songId !== songId || data.representation !== 'spotify_three_band' || data.displayAggregation !== 'signed_absolute_max_v1' || !Number.isInteger(data.totalSamples) || data.totalSamples <= 0 || !Number.isInteger(data.displayStride) || data.displayStride <= 0 || !Number.isFinite(data.durationSeconds) || data.durationSeconds <= 0 || !['duration_compatible', 'duration_mismatch', 'local_duration_unavailable'].includes(data.alignment)) throw new Error('Invalid provider waveform');
  for (const band of [data.lows, data.mids, data.highs]) {
    if (!Array.isArray(band) || band.length === 0 || band.length > 600 || band.length !== Math.ceil(data.totalSamples / data.displayStride)) throw new Error('Invalid provider waveform');
    for (const value of band) if (!Number.isInteger(value) || value < -2147483648 || value > 2147483647) throw new Error('Invalid provider waveform');
  }
  return data;
}
export function signedBandPaths(bands: number[][]): string[] {
  let peak = 0;
  for (const band of bands) for (const value of band) peak = Math.max(peak, Math.abs(value));
  return bands.map(band => band.map((value, i) => `${i === 0 ? 'M' : 'L'}${(band.length === 1 ? 0 : i * 600 / (band.length - 1)).toFixed(2)},${(40 - (peak === 0 ? 0 : value / peak * 35)).toFixed(2)}`).join(' '));
}
