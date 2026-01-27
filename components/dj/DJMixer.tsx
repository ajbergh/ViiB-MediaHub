/**
 * ViiB MediaHub - DJ Mixer Component
 * 
 * Central mixer section with:
 * - Crossfader
 * - Per-channel volume faders
 * - 3-band EQ per channel (High/Mid/Low)
 * - VU meters
 * - Master volume
 * 
 * @module components/dj/DJMixer
 */

import React, { useCallback } from 'react';
import { useStore } from '../../store';
import { useDJAudioEngine } from '../../hooks/useDJAudioEngine';
import type { DeckId, DeckEQ } from '../../slices/djMixerSlice';

// Headphone icon
const HeadphoneIcon: React.FC<{ className?: string }> = ({ className }) => (
  <svg className={className} width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <path d="M3 18v-6a9 9 0 0 1 18 0v6" />
    <path d="M21 19a2 2 0 0 1-2 2h-1a2 2 0 0 1-2-2v-3a2 2 0 0 1 2-2h3zM3 19a2 2 0 0 0 2 2h1a2 2 0 0 0 2-2v-3a2 2 0 0 0-2-2H3z" />
  </svg>
);

export const DJMixer: React.FC = () => {
  // Only subscribe to specific state needed, avoiding position updates
  const djDeckAEq = useStore(state => state.djDeckA.eq);
  const djDeckAVolume = useStore(state => state.djDeckA.volume);
  const djDeckAIsPlaying = useStore(state => state.djDeckA.isPlaying);
  const djDeckACueEnabled = useStore(state => state.djDeckA.cueEnabled);
  const djDeckBEq = useStore(state => state.djDeckB.eq);
  const djDeckBVolume = useStore(state => state.djDeckB.volume);
  const djDeckBIsPlaying = useStore(state => state.djDeckB.isPlaying);
  const djDeckBCueEnabled = useStore(state => state.djDeckB.cueEnabled);
  const crossfader = useStore(state => state.djMixer.crossfader);
  const masterVolume = useStore(state => state.djMixer.masterVolume);
  const headphoneVolume = useStore(state => state.djMixer.headphoneVolume);
  const headphoneMix = useStore(state => state.djMixer.headphoneMix);
  const syncMode = useStore(state => state.djMixer.syncMode);
  const setSyncMode = useStore(state => state.setSyncMode);
  const resetDeckEQ = useStore(state => state.resetDeckEQ);
  
  // Use audio engine hook for mixer controls (updates both audio and state)
  const { 
    setCrossfader, 
    setMasterVolume, 
    setVolume, 
    setEQ,
    toggleCue,
    setHeadphoneVolume,
    setHeadphoneMix,
  } = useDJAudioEngine();

  const handleCrossfaderChange = useCallback((e: React.ChangeEvent<HTMLInputElement>) => {
    setCrossfader(parseFloat(e.target.value));
  }, [setCrossfader]);

  const handleMasterVolumeChange = useCallback((e: React.ChangeEvent<HTMLInputElement>) => {
    setMasterVolume(parseFloat(e.target.value));
  }, [setMasterVolume]);
  
  const handleEQChange = useCallback((deck: DeckId, band: keyof DeckEQ, value: number) => {
    setEQ(deck, band, value);
  }, [setEQ]);

  const handleHeadphoneVolumeChange = useCallback((e: React.ChangeEvent<HTMLInputElement>) => {
    setHeadphoneVolume(parseFloat(e.target.value));
  }, [setHeadphoneVolume]);

  const handleHeadphoneMixChange = useCallback((e: React.ChangeEvent<HTMLInputElement>) => {
    setHeadphoneMix(parseFloat(e.target.value));
  }, [setHeadphoneMix]);

  return (
    <div className="h-full flex flex-col p-3 gap-2 overflow-y-auto">
      {/* Header */}
      <div className="text-center flex-shrink-0">
        <span className="text-xs uppercase tracking-wider text-neutral-500">Mixer</span>
      </div>

      {/* Sync Mode Selector (Phase 4) */}
      <div className="flex-shrink-0 space-y-1">
        <div className="text-[10px] text-neutral-500 text-center">Sync Mode</div>
        <div className="flex justify-center gap-1">
          <button
            onClick={() => setSyncMode('off')}
            className={`px-2 py-0.5 text-[9px] font-bold rounded transition-colors
              ${syncMode === 'off' 
                ? 'bg-red-500/80 text-white' 
                : 'bg-surface-2 text-neutral-400 hover:text-white hover:bg-surface-1'
              }`}
          >
            OFF
          </button>
          <button
            onClick={() => setSyncMode('bpm')}
            className={`px-2 py-0.5 text-[9px] font-bold rounded transition-colors
              ${syncMode === 'bpm' 
                ? 'bg-brand text-white' 
                : 'bg-surface-2 text-neutral-400 hover:text-white hover:bg-surface-1'
              }`}
          >
            BPM
          </button>
          <button
            onClick={() => setSyncMode('beat-phase')}
            className={`px-2 py-0.5 text-[9px] font-bold rounded transition-colors
              ${syncMode === 'beat-phase' 
                ? 'bg-amber-500 text-white' 
                : 'bg-surface-2 text-neutral-400 hover:text-white hover:bg-surface-1'
              }`}
            title="Sync BPM + Beat Phase"
          >
            PHASE
          </button>
        </div>
      </div>

      {/* EQ Section - more compact */}
      <div className="flex gap-3 flex-shrink-0">
        {/* Channel A EQ */}
        <EQChannel deck="A" eq={djDeckAEq} onEQChange={handleEQChange} onReset={resetDeckEQ} />

        {/* Channel B EQ */}
        <EQChannel deck="B" eq={djDeckBEq} onEQChange={handleEQChange} onReset={resetDeckEQ} />
      </div>

      {/* Volume Faders - compact */}
      <div className="flex gap-4 justify-center flex-shrink-0">
        <VolumeFader
          deck="A"
          volume={djDeckAVolume}
          isPlaying={djDeckAIsPlaying}
          onChange={(v) => setVolume('A', v)}
        />
        <VolumeFader
          deck="B"
          volume={djDeckBVolume}
          isPlaying={djDeckBIsPlaying}
          onChange={(v) => setVolume('B', v)}
        />
      </div>

      {/* Crossfader - ensure always visible */}
      <div className="space-y-1 flex-shrink-0 mt-auto pt-2 border-t border-white/5">
        <div className="flex justify-between text-[10px] text-neutral-500">
          <span>A</span>
          <span className="font-medium">Crossfader</span>
          <span>B</span>
        </div>
        <input
          type="range"
          min="-1"
          max="1"
          step="0.01"
          value={crossfader}
          onChange={handleCrossfaderChange}
          className="w-full h-4 bg-surface-2 rounded-full appearance-none cursor-pointer
                     [&::-webkit-slider-thumb]:appearance-none
                     [&::-webkit-slider-thumb]:w-8
                     [&::-webkit-slider-thumb]:h-5
                     [&::-webkit-slider-thumb]:rounded-md
                     [&::-webkit-slider-thumb]:bg-brand
                     [&::-webkit-slider-thumb]:cursor-grab
                     [&::-webkit-slider-thumb]:active:cursor-grabbing
                     [&::-webkit-slider-thumb]:shadow-md"
        />
        {/* Center marker */}
        <div className="flex justify-center">
          <div className="w-0.5 h-1.5 bg-neutral-600" />
        </div>
      </div>

      {/* Master Volume - compact */}
      <div className="space-y-1 flex-shrink-0">
        <div className="flex justify-between text-[10px] text-neutral-500">
          <span>Master</span>
          <span className="font-mono">{Math.round(masterVolume * 100)}%</span>
        </div>
        <input
          type="range"
          min="0"
          max="1"
          step="0.01"
          value={masterVolume}
          onChange={handleMasterVolumeChange}
          className="w-full h-2 accent-brand"
        />
      </div>

      {/* Headphone Cue Section (Phase 4) */}
      <div className="space-y-2 flex-shrink-0 pt-2 border-t border-white/5">
        <div className="flex items-center gap-1 text-[10px] text-neutral-500">
          <HeadphoneIcon className="w-3 h-3" />
          <span>Headphones</span>
        </div>
        
        {/* CUE Buttons */}
        <div className="flex justify-center gap-4">
          <button
            onClick={() => toggleCue('A')}
            className={`px-3 py-1 text-[10px] font-bold rounded transition-colors
              ${djDeckACueEnabled 
                ? 'bg-brand text-white shadow-lg shadow-brand/30' 
                : 'bg-surface-2 text-neutral-400 hover:text-white hover:bg-surface-1'
              }`}
          >
            CUE A
          </button>
          <button
            onClick={() => toggleCue('B')}
            className={`px-3 py-1 text-[10px] font-bold rounded transition-colors
              ${djDeckBCueEnabled 
                ? 'bg-brand text-white shadow-lg shadow-brand/30' 
                : 'bg-surface-2 text-neutral-400 hover:text-white hover:bg-surface-1'
              }`}
          >
            CUE B
          </button>
        </div>

        {/* Cue/Master Mix */}
        <div className="space-y-1">
          <div className="flex justify-between text-[9px] text-neutral-500">
            <span>CUE</span>
            <span>MIX</span>
            <span>MSTR</span>
          </div>
          <input
            type="range"
            min="0"
            max="1"
            step="0.01"
            value={headphoneMix}
            onChange={handleHeadphoneMixChange}
            className="w-full h-1.5 accent-amber-500"
          />
        </div>

        {/* Headphone Volume */}
        <div className="space-y-1">
          <div className="flex justify-between text-[9px] text-neutral-500">
            <span>Vol</span>
            <span className="font-mono">{Math.round(headphoneVolume * 100)}%</span>
          </div>
          <input
            type="range"
            min="0"
            max="1"
            step="0.01"
            value={headphoneVolume}
            onChange={handleHeadphoneVolumeChange}
            className="w-full h-1.5 accent-amber-500"
          />
        </div>
      </div>
    </div>
  );
};

// EQ Channel Component
interface EQChannelProps {
  deck: DeckId;
  eq: DeckEQ;
  onEQChange: (deck: DeckId, band: keyof DeckEQ, value: number) => void;
  onReset: (deck: DeckId) => void;
}

const EQChannel: React.FC<EQChannelProps> = ({ deck, eq, onEQChange, onReset }) => {
  const bands: (keyof DeckEQ)[] = ['high', 'mid', 'low'];
  const bandLabels = { high: 'HI', mid: 'MID', low: 'LOW' };
  const bandColors = { high: 'text-cyan-400', mid: 'text-green-400', low: 'text-amber-400' };

  return (
    <div className="flex-1 flex flex-col items-center gap-1">
      <div className="text-[10px] font-bold text-neutral-400">{deck}</div>
      
      {bands.map(band => (
        <div key={band} className="w-full">
          <div className={`text-[9px] text-center ${bandColors[band]}`}>
            {bandLabels[band]}
          </div>
          <EQKnob
            value={eq[band]}
            onChange={(v) => onEQChange(deck, band, v)}
          />
        </div>
      ))}

      {/* Reset button */}
      <button
        onClick={() => onReset(deck)}
        className="text-[9px] text-neutral-500 hover:text-neutral-300 px-1"
      >
        Reset
      </button>
    </div>
  );
};

// EQ Knob (simplified as vertical slider for MVP)
interface EQKnobProps {
  value: number;
  onChange: (value: number) => void;
}

const EQKnob: React.FC<EQKnobProps> = ({ value, onChange }) => {
  const handleChange = useCallback((e: React.ChangeEvent<HTMLInputElement>) => {
    onChange(parseFloat(e.target.value));
  }, [onChange]);

  // Double-click to reset to 0
  const handleDoubleClick = useCallback(() => {
    onChange(0);
  }, [onChange]);

  const displayValue = value === 0 ? '0' : value > 0 ? `+${value}` : `${value}`;

  return (
    <div className="flex flex-col items-center">
      <input
        type="range"
        min="-24"
        max="12"
        step="1"
        value={value}
        onChange={handleChange}
        onDoubleClick={handleDoubleClick}
        className="h-16 w-5 accent-brand appearance-none cursor-pointer writing-mode-vertical
                   [writing-mode:vertical-lr]
                   [-webkit-appearance:slider-vertical]"
        style={{ writingMode: 'vertical-lr' } as any}
      />
      <span className="text-[9px] font-mono text-neutral-400 w-6 text-center">
        {displayValue}
      </span>
    </div>
  );
};

// Volume Fader Component
interface VolumeFaderProps {
  deck: DeckId;
  volume: number;
  isPlaying: boolean;
  onChange: (volume: number) => void;
}

const VolumeFader: React.FC<VolumeFaderProps> = ({ deck, volume, isPlaying, onChange }) => {
  const handleChange = useCallback((e: React.ChangeEvent<HTMLInputElement>) => {
    onChange(parseFloat(e.target.value));
  }, [onChange]);

  return (
    <div className="flex flex-col items-center gap-1">
      <div className={`text-[10px] font-bold ${isPlaying ? 'text-brand' : 'text-neutral-500'}`}>
        {deck}
      </div>
      <input
        type="range"
        min="0"
        max="1"
        step="0.01"
        value={volume}
        onChange={handleChange}
        className="h-16 w-5 accent-brand appearance-none cursor-pointer
                   [writing-mode:vertical-lr]
                   [-webkit-appearance:slider-vertical]"
        style={{ writingMode: 'vertical-lr' } as any}
      />
      <span className="text-[9px] font-mono text-neutral-400">
        {Math.round(volume * 100)}
      </span>
    </div>
  );
};

export default DJMixer;
