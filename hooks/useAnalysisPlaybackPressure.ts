/**
 * useAnalysisPlaybackPressure tells the backend that DJ playback is active so
 * background track analysis yields its worker instead of competing for CPU
 * during a set.
 *
 * The signal is a renewed TTL rather than a sticky flag on purpose: if the
 * window closes, crashes, or loses its connection mid-set, the report expires
 * on its own and the library queue resumes. A "playback stopped" message that
 * never arrives must not park analysis forever.
 */
import { useEffect, useRef } from 'react';
import { jobsV2 } from '../services/jobsV2';
import { useStore } from '../store';

/** How long one report suppresses background analysis. */
const PRESSURE_TTL_SECONDS = 30;

/** Renew well before expiry so a slow request cannot create a gap. */
const RENEW_INTERVAL_MS = 10_000;

export const useAnalysisPlaybackPressure = () => {
  const deckAPlaying = useStore(state => state.djDeckA.isPlaying);
  const deckBPlaying = useStore(state => state.djDeckB.isPlaying);
  const playing = deckAPlaying || deckBPlaying;

  // Tracks whether the backend currently believes playback is active, so
  // stopping only sends one clear rather than one per render.
  const reported = useRef(false);

  useEffect(() => {
    let disposed = false;

    const report = (active: boolean) => {
      // Analysis pressure is a hint. A failed report must never surface as a
      // user-visible error or interrupt playback.
      void jobsV2.reportPlaybackPressure(active, active ? PRESSURE_TTL_SECONDS : undefined)
        .catch(() => undefined);
    };

    if (!playing) {
      if (reported.current) {
        reported.current = false;
        report(false);
      }
      return;
    }

    reported.current = true;
    report(true);
    const timer = window.setInterval(() => {
      if (!disposed) report(true);
    }, RENEW_INTERVAL_MS);

    return () => {
      disposed = true;
      window.clearInterval(timer);
    };
  }, [playing]);

  // Leaving DJ mode while a deck is still playing must not leave a stale
  // report behind; the TTL would cover it, but clearing is immediate.
  useEffect(() => () => {
    if (reported.current) {
      reported.current = false;
      void jobsV2.reportPlaybackPressure(false).catch(() => undefined);
    }
  }, []);
};
