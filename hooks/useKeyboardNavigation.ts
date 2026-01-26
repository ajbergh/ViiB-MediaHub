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
 * - N: Toggle Now Playing view
 * - V: Cycle visualizer mode (in Now Playing)
 * - ]: Next Milkdrop preset (when in Milkdrop mode)
 * - [: Previous Milkdrop preset (when in Milkdrop mode) 
 * - F: Toggle favorite for current Milkdrop preset
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
    // Milkdrop state
    audioSettings,
    setVisualizerMode,
    milkdropSettings,
    setMilkdropPreset,
    toggleMilkdropFavorite,
    milkdropPresetKeys,
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

      // Milkdrop shortcuts (only active in Now Playing with Milkdrop mode)
      case 'v':
      case 'V':
        // Cycle visualizer mode when in Now Playing
        if (isNowPlayingOpen) {
          const modes = ['OFF', 'WAVE', 'SPECTRUM', 'FLAME_SPECTRUM', 'STARDUST_HALO', 
            'AURORA_RIBBON', 'ELECTRIC_ARC', 'GRASS_OSCILLOSCOPE', 'FIREFLY_FIELD',
            'TUNNEL_WAVEFORM', 'WIND_FIELD', 'MILKDROP'] as const;
          const currentIdx = modes.indexOf(audioSettings.visualizerMode as typeof modes[number]);
          const nextIdx = (currentIdx + 1) % modes.length;
          setVisualizerMode(modes[nextIdx]);
        }
        break;

      case ']':
        // Next Milkdrop preset
        if (isNowPlayingOpen && audioSettings.visualizerMode === 'MILKDROP' && milkdropPresetKeys.length > 0) {
          const currentIdx = milkdropSettings.currentPreset 
            ? milkdropPresetKeys.indexOf(milkdropSettings.currentPreset)
            : -1;
          const nextIdx = (currentIdx + 1) % milkdropPresetKeys.length;
          setMilkdropPreset(milkdropPresetKeys[nextIdx]);
        }
        break;

      case '[':
        // Previous Milkdrop preset
        if (isNowPlayingOpen && audioSettings.visualizerMode === 'MILKDROP' && milkdropPresetKeys.length > 0) {
          const currentIdx = milkdropSettings.currentPreset 
            ? milkdropPresetKeys.indexOf(milkdropSettings.currentPreset)
            : 0;
          const prevIdx = (currentIdx - 1 + milkdropPresetKeys.length) % milkdropPresetKeys.length;
          setMilkdropPreset(milkdropPresetKeys[prevIdx]);
        }
        break;

      case 'f':
      case 'F':
        // Toggle favorite for current Milkdrop preset
        if (isNowPlayingOpen && audioSettings.visualizerMode === 'MILKDROP' && milkdropSettings.currentPreset) {
          toggleMilkdropFavorite(milkdropSettings.currentPreset);
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
    audioSettings,
    setVisualizerMode,
    milkdropSettings,
    setMilkdropPreset,
    toggleMilkdropFavorite,
    milkdropPresetKeys,
  ]);

  useEffect(() => {
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [handleKeyDown]);
}

export default useKeyboardNavigation;
