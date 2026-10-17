import { useEffect, useState } from 'react';
import { useStore } from '../store';
import { api } from '../services/api';
import { loadTrackThreeBand, type TrackThreeBand } from '../services/trackThreeBand';
import { reportAudioReadFailure, withSharedAudioMetadataRead } from '../services/audioReadDiagnostics';
import { djTrackSourceIdentity } from '../lib/djBpmCorrection';
import type { DeckId } from '../slices/djMixerSlice';
import type { Song } from '../types';

export interface DJThreeBands { data: TrackThreeBand; peak: number }
export interface DJBandSnapshot { bands: DJThreeBands; track: Song | null; session: number; key: string }

export function currentDJThreeBands(snapshot: DJBandSnapshot | undefined, track: Song | null, duration: number, session: number): DJThreeBands | undefined {
  return snapshot && snapshot.track === track && snapshot.session === session && duration > 0
    && Math.abs(snapshot.bands.data.durationSeconds - duration) <= 0.1 ? snapshot.bands : undefined;
}

/** One source/session ownership contract for both GPU and Canvas renderers. */
export function useDJThreeBands(deck: DeckId, enabled: boolean, retryGeneration = 0): DJBandSnapshot | undefined {
  const track = useStore(s => (deck === 'A' ? s.djDeckA : s.djDeckB).track);
  const duration = useStore(s => (deck === 'A' ? s.djDeckA : s.djDeckB).duration);
  const session = useStore(s => s.spotifySessionGeneration);
  const metadataKey = JSON.stringify([track?.id, djTrackSourceIdentity(track), session]);
  const key = JSON.stringify([metadataKey, retryGeneration]);
  const [loaded, setLoaded] = useState<DJBandSnapshot>();
  useEffect(() => {
    let active = true;
    let readingMetadata = true;
    if (enabled && track) void (async () => {
      const metadata = await withSharedAudioMetadataRead(metadataKey, () => api.getTrackAnalysisFeature(track.id), () => active);
      if (!active || !metadata.sourceFingerprint) return;
      readingMetadata = false;
      const result = await loadTrackThreeBand(track.id, metadata.sourceFingerprint, session, () => active);
      if (!active) return;
      if (result.providerError) reportAudioReadFailure('provider_bands', result.providerError);
      if (result.localError) reportAudioReadFailure('local_bands', result.localError);
      if (!result.data) return;
      let peak = 0;
      for (const band of [result.data.low, result.data.mid, result.data.high]) {
        for (const value of band) peak = Math.max(peak, value);
      }
      setLoaded({ track, key, session, bands: { data: result.data, peak } });
    })().catch(error => { if (active) reportAudioReadFailure(readingMetadata ? 'audio_metadata' : 'local_bands', error); });
    return () => { active = false; };
  }, [enabled, track, key, metadataKey, session]);
  return enabled && loaded?.key === key && currentDJThreeBands(loaded, track, duration, session) ? loaded : undefined;
}
