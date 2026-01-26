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

export const DJMixer: React.FC = () => {
  const {
    djDeckA,
    djDeckB,
    djMixer,
    resetDeckEQ,
  } = useStore();
  
  // Use audio engine hook for mixer controls (updates both audio and state)
  const { setCrossfader, setMasterVolume, setVolume, setEQ } = useDJAudioEngine();

  const handleCrossfaderChange = useCallback((e: React.ChangeEvent<HTMLInputElement>) => {
    setCrossfader(parseFloat(e.target.value));
  }, [setCrossfader]);

  const handleMasterVolumeChange = useCallback((e: React.ChangeEvent<HTMLInputElement>) => {
    setMasterVolume(parseFloat(e.target.value));
  }, [setMasterVolume]);
  
  const handleEQChange = useCallback((deck: DeckId, band: keyof DeckEQ, value: number) => {
    setEQ(deck, band, value);
  }, [setEQ]);

  return (
    <div className="h-full flex flex-col p-4 gap-4">
      {/* Header */}
      <div className="text-center">
        <span className="text-xs uppercase tracking-wider text-neutral-500">Mixer</span>
      </div>

      {/* EQ Section */}
      <div className="flex gap-4 flex-1">
        {/* Channel A EQ */}
        <EQChannel deck="A" eq={djDeckA.eq} onEQChange={handleEQChange} onReset={resetDeckEQ} />

        {/* Channel B EQ */}
        <EQChannel deck="B" eq={djDeckB.eq} onEQChange={handleEQChange} onReset={resetDeckEQ} />
      </div>

      {/* Volume Faders */}
      <div className="flex gap-4 justify-center">
        <VolumeFader
          deck="A"
          volume={djDeckA.volume}
          isPlaying={djDeckA.isPlaying}
          onChange={(v) => setVolume('A', v)}
        />
        <VolumeFader
          deck="B"
          volume={djDeckB.volume}
          isPlaying={djDeckB.isPlaying}
          onChange={(v) => setVolume('B', v)}
        />
      </div>

      {/* Crossfader */}
      <div className="space-y-2">
        <div className="flex justify-between text-xs text-neutral-500">
          <span>A</span>
          <span>Crossfader</span>
          <span>B</span>
        </div>
        <input
          type="range"
          min="-1"
          max="1"
          step="0.01"
          value={djMixer.crossfader}
          onChange={handleCrossfaderChange}
          className="w-full h-3 bg-surface-2 rounded-full appearance-none cursor-pointer
                     [&::-webkit-slider-thumb]:appearance-none
                     [&::-webkit-slider-thumb]:w-6
                     [&::-webkit-slider-thumb]:h-6
                     [&::-webkit-slider-thumb]:rounded-full
                     [&::-webkit-slider-thumb]:bg-brand
                     [&::-webkit-slider-thumb]:cursor-grab
                     [&::-webkit-slider-thumb]:active:cursor-grabbing"
        />
        {/* Center marker */}
        <div className="flex justify-center">
          <div className="w-0.5 h-2 bg-neutral-600" />
        </div>
      </div>

      {/* Master Volume */}
      <div className="space-y-1">
        <div className="flex justify-between text-xs text-neutral-500">
          <span>Master</span>
          <span>{Math.round(djMixer.masterVolume * 100)}%</span>
        </div>
        <input
          type="range"
          min="0"
          max="1"
          step="0.01"
          value={djMixer.masterVolume}
          onChange={handleMasterVolumeChange}
          className="w-full accent-brand"
        />
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
    <div className="flex-1 flex flex-col items-center gap-2">
      <div className="text-xs font-bold text-neutral-400">{deck}</div>
      
      {bands.map(band => (
        <div key={band} className="w-full space-y-1">
          <div className={`text-xs text-center ${bandColors[band]}`}>
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
        className="text-xs text-neutral-500 hover:text-neutral-300 mt-1"
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
    <div className="flex flex-col items-center gap-1">
      <input
        type="range"
        min="-24"
        max="12"
        step="1"
        value={value}
        onChange={handleChange}
        onDoubleClick={handleDoubleClick}
        className="h-20 w-6 accent-brand appearance-none cursor-pointer writing-mode-vertical
                   [writing-mode:vertical-lr]
                   [-webkit-appearance:slider-vertical]"
        style={{ writingMode: 'vertical-lr' } as any}
      />
      <span className="text-xs font-mono text-neutral-400 w-8 text-center">
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
    <div className="flex flex-col items-center gap-2">
      <div className={`text-xs font-bold ${isPlaying ? 'text-brand' : 'text-neutral-500'}`}>
        {deck}
      </div>
      <input
        type="range"
        min="0"
        max="1"
        step="0.01"
        value={volume}
        onChange={handleChange}
        className="h-24 w-6 accent-brand appearance-none cursor-pointer
                   [writing-mode:vertical-lr]
                   [-webkit-appearance:slider-vertical]"
        style={{ writingMode: 'vertical-lr' } as any}
      />
      <span className="text-xs font-mono text-neutral-400">
        {Math.round(volume * 100)}
      </span>
    </div>
  );
};

export default DJMixer;
