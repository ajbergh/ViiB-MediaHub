/**
 * ViiB MediaHub - DJ Sampler Pads Component
 * 
 * 8 trigger pads for one-shot samples, loops, and sound effects.
 * Each pad can load audio files, play in oneshot/loop/gate modes,
 * and has individual volume control.
 * 
 * @module components/dj/v2/DJSamplerPads
 */

import React, { useCallback, useRef, memo } from 'react';
import { useStore } from '../../../store';
import { getDJSamplerEngine } from '../../../lib/djSampler';
import type { SamplerPad } from '../../../slices/djMixerSlice';

// ============================================================================
// Sampler Pad Button
// ============================================================================

const SamplerPadButton = memo(({
  pad,
  onTrigger,
  onStop,
  onLoad,
  onClear,
  onVolumeChange,
  onModeChange,
}: {
  pad: SamplerPad;
  onTrigger: (id: number) => void;
  onStop: (id: number) => void;
  onLoad: (id: number) => void;
  onClear: (id: number) => void;
  onVolumeChange: (id: number, volume: number) => void;
  onModeChange: (id: number) => void;
}) => {
  const hasAudio = pad.url !== null;
  
  const handleMouseDown = useCallback(() => {
    if (!hasAudio) return;
    onTrigger(pad.id);
  }, [hasAudio, pad.id, onTrigger]);
  
  const handleMouseUp = useCallback(() => {
    if (pad.mode === 'gate' && pad.isPlaying) {
      onStop(pad.id);
    }
  }, [pad.mode, pad.isPlaying, pad.id, onStop]);
  
  const handleClick = useCallback(() => {
    if (!hasAudio) {
      onLoad(pad.id);
      return;
    }
    // For non-gate modes, click toggles play/stop
    if (pad.mode !== 'gate') {
      if (pad.isPlaying) {
        onStop(pad.id);
      } else {
        onTrigger(pad.id);
      }
    }
  }, [hasAudio, pad.mode, pad.isPlaying, pad.id, onLoad, onTrigger, onStop]);

  const handleContextMenu = useCallback((e: React.MouseEvent) => {
    e.preventDefault();
    if (hasAudio) {
      onClear(pad.id);
    }
  }, [hasAudio, pad.id, onClear]);
  
  const modeLabel = pad.mode === 'oneshot' ? '1×' : pad.mode === 'loop' ? '∞' : '⏏';
  
  return (
    <div className="flex flex-col items-center gap-0.5">
      {/* Pad button */}
      <button
        className={`w-14 h-12 rounded-md border-2 transition-all duration-75 flex flex-col items-center justify-center text-[9px] font-bold select-none ${
          pad.isPlaying
            ? 'shadow-lg scale-[0.97]'
            : hasAudio
              ? 'hover:brightness-125 active:scale-95'
              : 'border-dashed opacity-50 hover:opacity-75'
        }`}
        style={{
          borderColor: hasAudio ? pad.color : '#555',
          backgroundColor: pad.isPlaying
            ? `${pad.color}40`
            : hasAudio
              ? `${pad.color}15`
              : 'transparent',
          boxShadow: pad.isPlaying ? `0 0 12px ${pad.color}60` : 'none',
        }}
        onClick={pad.mode === 'gate' ? undefined : handleClick}
        onMouseDown={pad.mode === 'gate' ? handleMouseDown : undefined}
        onMouseUp={pad.mode === 'gate' ? handleMouseUp : undefined}
        onMouseLeave={pad.mode === 'gate' && pad.isPlaying ? () => onStop(pad.id) : undefined}
        onContextMenu={handleContextMenu}
        title={hasAudio ? `${pad.name} (right-click to clear)` : 'Click to load sample'}
      >
        <span 
          className="truncate w-full text-center px-1"
          style={{ color: hasAudio ? pad.color : '#666' }}
        >
          {hasAudio ? pad.name : '+'}
        </span>
        {hasAudio && (
          <span className="text-[7px] text-neutral-500 mt-0.5">
            {pad.id + 1}
          </span>
        )}
      </button>
      
      {/* Controls row: mode + volume */}
      {hasAudio && (
        <div className="flex items-center gap-1">
          <button
            className="text-[8px] px-1 py-0.5 rounded bg-neutral-800 hover:bg-neutral-700 text-neutral-400"
            onClick={() => onModeChange(pad.id)}
            title={`Mode: ${pad.mode} (click to cycle)`}
          >
            {modeLabel}
          </button>
          <input
            type="range"
            min={0}
            max={1}
            step={0.05}
            value={pad.volume}
            onChange={(e) => onVolumeChange(pad.id, parseFloat(e.target.value))}
            className="w-10 h-1 accent-neutral-500"
            title={`Volume: ${Math.round(pad.volume * 100)}%`}
          />
        </div>
      )}
    </div>
  );
});
SamplerPadButton.displayName = 'SamplerPadButton';

// ============================================================================
// Main Sampler Pads Component
// ============================================================================

export const DJSamplerPads: React.FC = memo(() => {
  const djSampler = useStore(state => state.djSampler);
  const loadSamplerPad = useStore(state => state.loadSamplerPad);
  const clearSamplerPad = useStore(state => state.clearSamplerPad);
  const setSamplerPadVolume = useStore(state => state.setSamplerPadVolume);
  const setSamplerPadMode = useStore(state => state.setSamplerPadMode);
  const setSamplerPadPlaying = useStore(state => state.setSamplerPadPlaying);
  
  const fileInputRef = useRef<HTMLInputElement>(null);
  const pendingPadId = useRef<number>(0);
  
  const engine = getDJSamplerEngine();
  
  const handleTrigger = useCallback((padId: number) => {
    engine.triggerPad(padId);
  }, [engine]);
  
  const handleStop = useCallback((padId: number) => {
    engine.stopPad(padId);
  }, [engine]);
  
  const handleLoad = useCallback((padId: number) => {
    pendingPadId.current = padId;
    fileInputRef.current?.click();
  }, []);
  
  const handleClear = useCallback((padId: number) => {
    engine.clearSample(padId);
    clearSamplerPad(padId);
  }, [engine, clearSamplerPad]);
  
  const handleVolumeChange = useCallback((padId: number, volume: number) => {
    setSamplerPadVolume(padId, volume);
    engine.setVolume(padId, volume);
  }, [setSamplerPadVolume, engine]);
  
  const handleModeChange = useCallback((padId: number) => {
    const pad = djSampler[padId];
    if (!pad) return;
    const modes: Array<'oneshot' | 'loop' | 'gate'> = ['oneshot', 'loop', 'gate'];
    const nextIdx = (modes.indexOf(pad.mode) + 1) % modes.length;
    setSamplerPadMode(padId, modes[nextIdx]);
  }, [djSampler, setSamplerPadMode]);
  
  const handleFileSelect = useCallback(async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    
    const padId = pendingPadId.current;
    const url = URL.createObjectURL(file);
    const name = file.name.replace(/\.[^.]+$/, '').substring(0, 12);
    
    const loaded = await engine.loadSample(padId, url);
    if (loaded) {
      loadSamplerPad(padId, name, url);
    }
    
    // Reset input to allow re-selecting same file
    e.target.value = '';
  }, [engine, loadSamplerPad]);
  
  return (
    <div className="flex flex-col gap-1">
      <div className="text-[9px] text-neutral-500 font-bold uppercase tracking-wider px-1">
        Sampler
      </div>
      <div className="grid grid-cols-4 gap-1">
        {djSampler.map(pad => (
          <SamplerPadButton
            key={pad.id}
            pad={pad}
            onTrigger={handleTrigger}
            onStop={handleStop}
            onLoad={handleLoad}
            onClear={handleClear}
            onVolumeChange={handleVolumeChange}
            onModeChange={handleModeChange}
          />
        ))}
      </div>
      
      {/* Hidden file input for loading samples */}
      <input
        ref={fileInputRef}
        type="file"
        accept="audio/*"
        className="hidden"
        onChange={handleFileSelect}
      />
    </div>
  );
});
DJSamplerPads.displayName = 'DJSamplerPads';

export default DJSamplerPads;
