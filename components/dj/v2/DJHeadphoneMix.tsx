import React, { useCallback, useRef, useEffect, useState } from 'react';
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
  const setHeadphoneMix = useStore(state => state.setHeadphoneMix);
  const setHeadphoneVolume = useStore(state => state.setHeadphoneVolume);
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
      {/* Label */}
      <div className="text-[9px] uppercase text-neutral-400 font-bold tracking-wider">
        Headphones
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
        {/* Track */}
        <div className="absolute inset-y-0 left-1 right-1 flex items-center">
          {/* Left label */}
          <span className="text-[7px] text-orange-400 font-bold">CUE</span>
          
          {/* Center line */}
          <div className="flex-1 flex justify-center">
            <div className="w-px h-3 bg-[#444]" />
          </div>
          
          {/* Right label */}
          <span className="text-[7px] text-cyan-400 font-bold">MST</span>
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

      {/* Mix Value */}
      <div className="text-[10px] font-mono text-neutral-300">
        {getMixLabel()}
      </div>

      {/* Volume Slider */}
      <div className="flex items-center gap-1 mt-1">
        <span className="text-[7px] text-neutral-500">VOL</span>
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
        <span className="text-[8px] font-mono text-neutral-400 w-6 text-right">
          {Math.round(volume * 100)}
        </span>
      </div>
    </div>
  );
};

export default DJHeadphoneMix;
