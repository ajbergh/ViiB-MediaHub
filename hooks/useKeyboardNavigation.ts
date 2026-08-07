import { useEffect } from 'react';
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
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      // Don't trigger shortcuts when typing in inputs
      const target = e.target as HTMLElement;
      if (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.isContentEditable) {
        // Allow Escape to blur inputs
        if (e.key === 'Escape') {
          target.blur();
        }
        return;
      }

      // Read current state at event time to avoid stale closures
      const state = useStore.getState();
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
        audioSettings,
        setVisualizerMode,
        milkdropSettings,
        setMilkdropPreset,
        toggleMilkdropFavorite,
        milkdropPresetKeys,
      } = state;

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
            prevSong();
          }
          break;

        case 'ArrowRight':
          if (e.shiftKey) {
            nextSong();
          }
          break;

        case 'ArrowUp':
          e.preventDefault();
          setVolume(Math.min(1, volume + 0.05));
          break;

        case 'ArrowDown':
          e.preventDefault();
          setVolume(Math.max(0, volume - 0.05));
          break;

        case 'm':
        case 'M':
          setVolume(volume > 0 ? 0 : 0.5);
          break;

        case 'q':
        case 'Q':
          setQueueOpen(!isQueueOpen);
          break;

        case 'e':
        case 'E':
          toggleEqPanel();
          break;

        case 'i':
        case 'I':
          if (currentSong && (e.ctrlKey || e.metaKey || isNowPlayingOpen)) {
            e.preventDefault();
            useStore.getState().openSongInfoModal(currentSong);
          }
          break;

        case 'n':
        case 'N':
          if (currentSong) {
            setNowPlayingOpen(!isNowPlayingOpen);
          }
          break;

        case 'v':
        case 'V':
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
          if (isNowPlayingOpen && audioSettings.visualizerMode === 'MILKDROP' && milkdropPresetKeys.length > 0) {
            const currentIdx = milkdropSettings.currentPreset 
              ? milkdropPresetKeys.indexOf(milkdropSettings.currentPreset)
              : -1;
            const nextIdx = (currentIdx + 1) % milkdropPresetKeys.length;
            setMilkdropPreset(milkdropPresetKeys[nextIdx]);
          }
          break;

        case '[':
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
          if (isNowPlayingOpen && audioSettings.visualizerMode === 'MILKDROP' && milkdropSettings.currentPreset) {
            toggleMilkdropFavorite(milkdropSettings.currentPreset);
          }
          break;

        case 'Escape':
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
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, []);
}

export default useKeyboardNavigation;
