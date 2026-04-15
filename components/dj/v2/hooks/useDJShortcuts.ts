/**
 * ViiB MediaHub - DJ Mode V2 Keyboard Shortcuts Hook
 *
 * Attaches a single keydown listener on mount using a stable ref pattern.
 * All callbacks are captured in a ref that stays current each render, so
 * the listener never needs to be removed/re-added when callback references
 * change — eliminating the churn documented in DJ_MODE_V2_SUGGESTIONS.md §2.2.
 *
 * @module components/dj/v2/hooks/useDJShortcuts
 */

import { useEffect, useRef } from 'react';
import { useStore } from '../../../../store';
import type { DeckId } from '../../../../slices/djMixerSlice';

const HOT_CUE_COLORS = [
  '#22c55e', '#22c55e', '#22c55e', '#eab308',
  '#f97316', '#3b82f6', '#8b5cf6', '#ec4899',
] as const;

export interface UseDJShortcutsOptions {
  togglePlay: (deck: DeckId) => void;
  returnToCue: (deck: DeckId) => void;
  setCrossfader: (value: number) => void;
  toggleActiveDeck: () => void;
  seek: (deck: DeckId, position: number) => void;
  setHotCue: (deck: DeckId, slot: number, position: number, label?: string, color?: string) => void;
  triggerHotCue: (deck: DeckId, slot: number) => void;
  handleSync: (deck: DeckId) => void;
  setShowShortcuts: React.Dispatch<React.SetStateAction<boolean>>;
}

/**
 * Register global DJ keyboard shortcuts.
 *
 * The event listener is added once on mount and never re-added, even when the
 * provided callbacks change identity between renders. This is achieved by
 * storing callbacks in a ref and updating the ref synchronously every render
 * (without a deps array on the second useEffect).
 */
export function useDJShortcuts(options: UseDJShortcutsOptions): void {
  // Always holds the latest version of every callback
  const callbacksRef = useRef(options);

  // Synchronously update the ref on every render — no deps needed
  useEffect(() => {
    callbacksRef.current = options;
  });

  // Attach the listener exactly once (empty dep array)
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      // Ignore when user is typing
      if (e.target instanceof HTMLInputElement || e.target instanceof HTMLTextAreaElement) return;

      const {
        togglePlay,
        returnToCue,
        setCrossfader,
        toggleActiveDeck,
        seek,
        setHotCue,
        triggerHotCue,
        handleSync,
        setShowShortcuts,
      } = callbacksRef.current;

      // Read current state snapshot — avoids any reactive subscriptions here
      const state = useStore.getState();
      const activeDeck = state.djActiveDeck;

      // Hot cue keys 1–8 (use e.code so Shift+1 doesn't become '!')
      const digitMatch = e.code.match(/^Digit([1-8])$/);
      if (digitMatch) {
        const slot = parseInt(digitMatch[1]);
        const deckState = activeDeck === 'A' ? state.djDeckA : state.djDeckB;
        if (!deckState.track) return;

        if (e.shiftKey) {
          const color = HOT_CUE_COLORS[slot - 1] ?? '#22c55e';
          setHotCue(activeDeck, slot, deckState.position, undefined, color);
        } else {
          const hotCue = deckState.hotCues.find(hc => hc.slot === slot);
          if (hotCue) {
            seek(activeDeck, hotCue.position);
            triggerHotCue(activeDeck, slot);
          }
        }
        return;
      }

      switch (e.key.toLowerCase()) {
        case 'q':        returnToCue('A'); break;
        case 'w':        togglePlay('A'); break;
        case 'o':        returnToCue('B'); break;
        case 'p':        togglePlay('B'); break;
        case ' ':
          e.preventDefault();
          togglePlay(activeDeck);
          break;
        case 'tab':
          e.preventDefault();
          toggleActiveDeck();
          break;
        case 'z':        setCrossfader(-1); break;
        case 'x':        setCrossfader(0); break;
        case 'c':        setCrossfader(1); break;
        case 'e':        handleSync('A'); break;
        case '[':        handleSync('B'); break;
        case 'arrowleft':
          if (e.shiftKey) setCrossfader(Math.max(-1, state.djMixer.crossfader - 0.1));
          break;
        case 'arrowright':
          if (e.shiftKey) setCrossfader(Math.min(1, state.djMixer.crossfader + 0.1));
          break;
        case 'f11':
          e.preventDefault();
          if (document.fullscreenElement) document.exitFullscreen();
          else document.documentElement.requestFullscreen();
          break;
        case '?':
          setShowShortcuts(prev => !prev);
          break;
        case '/': {
          e.preventDefault();
          const searchInput = document.querySelector(
            '[data-dj-mode] input[type="text"][placeholder*="Search"]',
          ) as HTMLInputElement | null;
          searchInput?.focus();
          break;
        }
        case 'escape':
          setShowShortcuts(false);
          break;
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, []); // ← intentionally empty: listener attaches once on mount
}
