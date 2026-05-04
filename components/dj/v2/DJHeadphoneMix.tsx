import React, { useCallback, useRef, useEffect, useState } from 'react';
import { Headphones } from 'lucide-react';
import { useStore } from '../../../store';

interface DJHeadphoneMixProps {
  width?: number;
  className?: string;
}

/**
 * DJ Headphone Mix Control
 * 
 * Knob control for blending between cue (PFL) and master signal
 * in the DJ's headphones. Allows previewing upcoming tracks
 * while still hearing the live mix.
 * 
 * - Full left (0): Cue only (hear only cued decks)
 * - Center (0.5): 50/50 mix of cue and master
 * - Full right (1): Master only (hear what audience hears)
 */
export const DJHeadphoneMix: React.FC<DJHeadphoneMixProps> = ({
  width = 120,
  className = '',
}) => {
  const mix = useStore(state => state.djMixer.headphoneMix);
  const volume = useStore(state => state.djMixer.headphoneVolume);
  const masterCueEnabled = useStore(state => state.djMixer.masterCueEnabled);
  const setHeadphoneMix = useStore(state => state.setHeadphoneMix);
  const setHeadphoneVolume = useStore(state => state.setHeadphoneVolume);
  const toggleMasterCue = useStore(state => state.toggleMasterCue);
  const sliderRef = useRef<HTMLDivElement>(null);
  const [isDragging, setIsDragging] = useState(false);

  const handleMixChange = useCallback((newMix: number) => {
    const clampedMix = Math.max(0, Math.min(1, newMix));
    setHeadphoneMix(clampedMix);
  }, [setHeadphoneMix]);

  const handleVolumeChange = useCallback((newVolume: number) => {
    const clampedVolume = Math.max(0, Math.min(1, newVolume));
    setHeadphoneVolume(clampedVolume);
  }, [setHeadphoneVolume]);

  const handleMasterCueToggle = useCallback(() => {
    toggleMasterCue();
  }, [toggleMasterCue]);

  // Handle slider drag
  useEffect(() => {
    if (!isDragging) return;

    const handleMouseMove = (e: MouseEvent) => {
      if (!sliderRef.current) return;
      const rect = sliderRef.current.getBoundingClientRect();
      const x = e.clientX - rect.left;
      const newMix = x / rect.width;
      handleMixChange(newMix);
    };

    const handleMouseUp = () => {
      setIsDragging(false);
    };

    document.addEventListener('mousemove', handleMouseMove);
    document.addEventListener('mouseup', handleMouseUp);
    return () => {
      document.removeEventListener('mousemove', handleMouseMove);
      document.removeEventListener('mouseup', handleMouseUp);
    };
  }, [isDragging, handleMixChange]);

  const handleMouseDown = (e: React.MouseEvent) => {
    setIsDragging(true);
    if (!sliderRef.current) return;
    const rect = sliderRef.current.getBoundingClientRect();
    const x = e.clientX - rect.left;
    const newMix = x / rect.width;
    handleMixChange(newMix);
  };

  // Double-click to reset to center
  const handleDoubleClick = () => {
    handleMixChange(0.5);
  };

  // Get label based on mix position
  const getMixLabel = () => {
    if (mix < 0.1) return 'CUE';
    if (mix > 0.9) return 'MST';
    return Math.round(mix * 100) + '%';
  };

  return (
    <div className={'flex flex-col items-center gap-1 ' + className}>
      {/* Label + master cue toggle */}
      <div className="flex items-center gap-2">
        <div className="text-[10px] uppercase text-neutral-400 font-bold tracking-wider">
          Headphones
        </div>
        <button
          type="button"
          onClick={handleMasterCueToggle}
          aria-pressed={masterCueEnabled}
          aria-label={`Master cue ${masterCueEnabled ? 'enabled' : 'disabled'}`}
          title="Master Cue - route master mix to headphones"
          className={`
            h-5 px-1.5 rounded border flex items-center gap-1 text-[9px] font-bold
            transition-all duration-100
            ${masterCueEnabled
              ? 'bg-cyan-600 text-white border-cyan-400 shadow shadow-cyan-500/30'
              : 'bg-[#222] text-neutral-500 border-[#333] hover:bg-[#2a2a2a] hover:text-cyan-300'}
          `}
        >
          <Headphones size={10} aria-hidden />
          <span>MST</span>
        </button>
      </div>
      
      {/* Mix Slider */}
      <div
        ref={sliderRef}
        className="relative h-5 bg-[#1a1a1a] rounded-full cursor-pointer border border-[#333]"
        style={{ width }}
        onMouseDown={handleMouseDown}
        onDoubleClick={handleDoubleClick}
        title="Headphone Mix - Cue ↔ Master (double-click to center)"
      >
        {/* Track — center line only, labels moved outside */}
        <div className="absolute inset-y-0 left-1 right-1 flex items-center justify-center">
          <div className="w-px h-3 bg-[#444]" />
        </div>
        
        {/* Thumb */}
        <div
          className="absolute top-0.5 w-3 h-4 rounded-sm bg-white shadow-lg shadow-white/20 transition-colors"
          style={{
            left: (width - 12) * mix,
            backgroundColor: mix < 0.5 ? '#f97316' : mix > 0.5 ? '#06b6d4' : '#fff',
          }}
        />
      </div>

      {/* CUE / value / MST row */}
      <div className="flex justify-between items-center" style={{ width }}>
        <span className="text-[9px] text-orange-400 font-bold">CUE</span>
        <div className="text-[10px] font-mono text-neutral-300">{getMixLabel()}</div>
        <span className="text-[9px] text-cyan-400 font-bold">MST</span>
      </div>

      {/* Volume Slider */}
      <div className="flex items-center gap-1 mt-1">
        <span className="text-[10px] text-neutral-500">VOL</span>
        <input
          type="range"
          min="0"
          max="1"
          step="0.01"
          value={volume}
          onChange={(e) => handleVolumeChange(parseFloat(e.target.value))}
          className="w-16 h-1 bg-[#333] rounded-full appearance-none cursor-pointer accent-orange-500"
          style={{ accentColor: '#f97316' }}
        />
        <span className="text-[10px] font-mono text-neutral-400 w-6 text-right">
          {Math.round(volume * 100)}
        </span>
      </div>
    </div>
  );
};

export default DJHeadphoneMix;
