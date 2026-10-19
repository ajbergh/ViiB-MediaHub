import { useCallback, useEffect, useRef, useState } from 'react';
import { DJFourChannelAudio, type DJFourChannelAudioOptions } from '../lib/djFourChannelAudio';

/** Start from a click/tap handler; connect sources only after start resolves. */
export function useFourChannelDJAudio() {
  const [engine, setEngine] = useState<DJFourChannelAudio | null>(null);
  const [starting, setStarting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const current = useRef<DJFourChannelAudio | null>(null);
  const pending = useRef<Promise<DJFourChannelAudio | null> | null>(null);
  const cancellation = useRef<AbortController | null>(null);
  const generation = useRef(0);
  const mounted = useRef(true);

  const stop = useCallback(() => {
    generation.current++;
    cancellation.current?.abort();
    cancellation.current = null;
    pending.current = null;
    const previous = current.current;
    current.current = null;
    setEngine(null);
    setStarting(false);
    // Closing the context also stops its connected sources.
    if (previous) void previous.dispose().catch(console.error);
  }, []);

  const start = useCallback((options: DJFourChannelAudioOptions = {}) => {
    if (current.current) return Promise.resolve(current.current);
    if (pending.current) return pending.current;
    const token = ++generation.current;
    const controller = new AbortController();
    cancellation.current = controller;
    setStarting(true);
    setError(null);
    const operation = DJFourChannelAudio.create(options, controller.signal).then(async created => {
      if (!mounted.current || generation.current !== token) {
        await created.dispose();
        return null;
      }
      current.current = created;
      setEngine(created);
      return created;
    }).catch((cause: unknown) => {
      if (mounted.current && generation.current === token) {
        setError(cause instanceof Error ? cause.message : String(cause));
      }
      return null;
    }).finally(() => {
      if (generation.current === token) {
        cancellation.current = null;
        pending.current = null;
        if (mounted.current) setStarting(false);
      }
    });
    pending.current = operation;
    return operation;
  }, []);

  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      generation.current++;
      cancellation.current?.abort();
      cancellation.current = null;
      pending.current = null;
      const previous = current.current;
      current.current = null;
      if (previous) void previous.dispose().catch(console.error);
    };
  }, []);

  // Stop, then await start(newOptions) to select a different device.
  return { engine, starting, error, start, stop };
}
