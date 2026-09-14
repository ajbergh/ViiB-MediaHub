import { useEffect, useRef, useState, type PointerEvent } from 'react';
import { useScratchAvailability } from './useScratchAvailability';
import { useStore } from '../store';
import { getDJAudioEngine } from '../lib/djAudio';
import { WaveformScratchGesture } from '../lib/waveformScratchGesture';
import type { DeckId } from '../slices/djMixerSlice';

/** Shared by the Canvas and WebGL timelines; scale is frozen for each grab. */
export function useWaveformScratch(deck: DeckId, visibleSeconds: number, enabled = true) {
  const trackId = useStore(state => (deck === 'A' ? state.djDeckA : state.djDeckB).track?.id);
  const availability = useScratchAvailability(deck);
  const [dragging, setDragging] = useState(false);
  const gesture = useRef<{ pointer: number; motion: WaveformScratchGesture; canvas: HTMLCanvasElement } | null>(null);
  const ownsScratch = useRef(false);

  useEffect(() => {
    setDragging(false);
    return () => {
      const active = gesture.current;
      gesture.current = null;
      if (ownsScratch.current) getDJAudioEngine().endScratch(deck, 0, true);
      ownsScratch.current = false;
      if (active?.canvas.hasPointerCapture(active.pointer)) active.canvas.releasePointerCapture(active.pointer);
    };
  }, [deck, trackId, enabled]);

  const finish = (event: PointerEvent<HTMLCanvasElement>) => {
    const active = gesture.current;
    if (!active || active.pointer !== event.pointerId) return;
    gesture.current = null;
    setDragging(false);
    getDJAudioEngine().endScratch(deck, active.motion.release(event.timeStamp, event.type !== 'pointerup'), true);
    if (event.currentTarget.hasPointerCapture(event.pointerId)) event.currentTarget.releasePointerCapture(event.pointerId);
  };

  return {
    className: `w-full select-none ${dragging ? 'cursor-grabbing' : availability.ready ? 'cursor-grab' : 'cursor-default'}`,
    title: 'Drag to scratch · Flick to coast · Double-click to seek',
    'aria-label': `Deck ${deck} waveform: drag to scratch, double-click to seek`,
    onPointerDown(event: PointerEvent<HTMLCanvasElement>) {
      const width = event.currentTarget.getBoundingClientRect().width;
      const engine = getDJAudioEngine();
      if (!enabled || !trackId || !engine.initialized || gesture.current || event.button !== 0 || width <= 0) return;
      if (!engine.startScratch(deck)) return;
      event.preventDefault();
      event.currentTarget.setPointerCapture(event.pointerId);
      gesture.current = { pointer: event.pointerId, canvas: event.currentTarget,
        motion: new WaveformScratchGesture(event.clientX, event.timeStamp, visibleSeconds / width) };
      ownsScratch.current = true;
      setDragging(true);
    },
    onPointerMove(event: PointerEvent<HTMLCanvasElement>) {
      const active = gesture.current;
      if (!active || active.pointer !== event.pointerId) return;
      event.preventDefault();
      const movement = active.motion.move(event.clientX, event.timeStamp);
      getDJAudioEngine().updateScratch(deck, movement.delta, movement.rate);
    },
    onPointerUp: finish,
    onPointerCancel: finish,
    onLostPointerCapture: finish,
  };
}
