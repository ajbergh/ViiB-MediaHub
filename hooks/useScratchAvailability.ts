import { useEffect, useState } from 'react';
import { getDJAudioEngine } from '../lib/djAudio';
import type { DeckId } from '../slices/djMixerSlice';

export function useScratchAvailability(deck: DeckId) {
  const [status, setStatus] = useState(() => getDJAudioEngine().getScratchStatus(deck));
  useEffect(() => {
    const update = () => setStatus(getDJAudioEngine().getScratchStatus(deck));
    update();
    const timer = window.setInterval(update, 200);
    return () => window.clearInterval(timer);
  }, [deck]);
  return { ready: status === 'Scratch', status };
}
