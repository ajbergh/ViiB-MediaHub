import { useEffect, useRef, useState } from 'react';

export interface ManualAudioMetadataChange {
  source: 'manual_audio_metadata';
  songId: string;
  sourceFingerprint: string;
  field: 'time_signature' | 'local_energy_level';
}
export function notifyAudioMetadataChange(change: ManualAudioMetadataChange) {
  window.dispatchEvent(new CustomEvent('library_updated', { detail: change }));
}

// Increment the ref synchronously so responses resolving before React cleanup
// cannot publish a snapshot captured before the invalidation.
export function useAudioMetadataRevision(songId?: string, energyOnly = false) {
  const [revision, setRevision] = useState(0);
  const generation = useRef(0);
  useEffect(() => {
    const changed = (event: Event) => {
      const detail = (event as CustomEvent<Partial<ManualAudioMetadataChange>>).detail;
      if (detail?.source !== 'manual_audio_metadata' || !detail.songId || !detail.sourceFingerprint
        || (detail.field !== 'time_signature' && detail.field !== 'local_energy_level')
        || (songId !== undefined && detail.songId !== songId)
        || (energyOnly && detail.field !== 'local_energy_level')) return;
      generation.current++;
      setRevision(value => value + 1);
    };
    window.addEventListener('library_updated', changed);
    return () => { window.removeEventListener('library_updated', changed); };
  }, [songId, energyOnly]);
  return { revision, generation };
}
