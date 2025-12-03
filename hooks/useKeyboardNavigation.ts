import { useEffect, useCallback } from 'react';
import { useStore } from '../store';

/**
 * Global keyboard navigation hook
 * Handles media controls and navigation shortcuts
 * 
 * Shortcuts:
 * - Space: Play/Pause
 * - Arrow Left: Seek backward 5s (Shift: Previous track)
 * - Arrow Right: Seek forward 5s (Shift: Next track)
 * - Arrow Up: Volume up 5%
 * - Arrow Down: Volume down 5%
 * - M: Toggle mute
 * - Q: Toggle queue panel
 * - E: Toggle equalizer panel
 * - Escape: Close panels
 */
export function useKeyboardNavigation() {
  const { 
    currentSong,
    togglePlay, 
    prevSong, 
    nextSong,
    setVolume,
    volume,
    isQueueOpen,
    setQueueOpen,
    isEqOpen,
    toggleEqPanel,
    isNowPlayingOpen,
    setNowPlayingOpen,
    closeContextMenu,
  } = useStore();

  const handleKeyDown = useCallback((e: KeyboardEvent) => {
    // Don't trigger shortcuts when typing in inputs
    const target = e.target as HTMLElement;
    if (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.isContentEditable) {
      // Allow Escape to blur inputs
      if (e.key === 'Escape') {
        target.blur();
      }
      return;
    }

    switch (e.key) {
      // Playback controls
      case ' ':
        if (currentSong) {
          e.preventDefault();
          togglePlay();
        }
        break;
      
      case 'ArrowLeft':
        if (e.shiftKey) {
          // Shift + Left: Previous track
          prevSong();
        }
        // Note: Seeking is handled by the audio element directly
        break;

      case 'ArrowRight':
        if (e.shiftKey) {
          // Shift + Right: Next track
          nextSong();
        }
        break;

      case 'ArrowUp':
        e.preventDefault();
        // Increase volume by 5%
        setVolume(Math.min(1, volume + 0.05));
        break;

      case 'ArrowDown':
        e.preventDefault();
        // Decrease volume by 5%
        setVolume(Math.max(0, volume - 0.05));
        break;

      case 'm':
      case 'M':
        // Toggle mute
        setVolume(volume > 0 ? 0 : 0.5);
        break;

      case 'q':
      case 'Q':
        // Toggle queue panel
        setQueueOpen(!isQueueOpen);
        break;

      case 'e':
      case 'E':
        // Toggle equalizer panel
        toggleEqPanel();
        break;

      case 'n':
      case 'N':
        // Toggle now playing view
        if (currentSong) {
          setNowPlayingOpen(!isNowPlayingOpen);
        }
        break;

      case 'Escape':
        // Close any open panels/menus
        if (isNowPlayingOpen) {
          setNowPlayingOpen(false);
        } else if (isQueueOpen) {
          setQueueOpen(false);
        } else if (isEqOpen) {
          toggleEqPanel();
        }
        closeContextMenu();
        break;
    }
  }, [
    currentSong,
    togglePlay, 
    prevSong, 
    nextSong, 
    volume, 
    setVolume, 
    isQueueOpen,
    setQueueOpen,
    isEqOpen,
    toggleEqPanel,
    isNowPlayingOpen,
    setNowPlayingOpen,
    closeContextMenu,
  ]);

  useEffect(() => {
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [handleKeyDown]);
}

export default useKeyboardNavigation;
