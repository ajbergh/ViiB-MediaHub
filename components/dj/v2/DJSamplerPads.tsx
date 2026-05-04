/**
 * ViiB MediaHub - DJ Sampler Pads Component
 * 
 * 8 trigger pads for one-shot samples, loops, and sound effects.
 * Each pad can load audio files, play in oneshot/loop/gate modes,
 * and has individual volume control.
 * 
 * @module components/dj/v2/DJSamplerPads
 */

import React, { useCallback, useEffect, useRef, useState, memo } from 'react';
import { useStore } from '../../../store';
import { getDJSamplerEngine } from '../../../lib/djSampler';
import type { SamplerPad } from '../../../slices/djMixerSlice';

const SAMPLER_METADATA_STORAGE_KEY = 'viib.dj.sampler.metadata';

type PersistedSamplerPad = {
  id: number;
  name: string;
  volume: number;
  mode: SamplerPad['mode'];
  color: string;
};

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
  progress,
  loading,
}: {
  pad: SamplerPad;
  onTrigger: (id: number) => void;
  onStop: (id: number) => void;
  onLoad: (id: number) => void;
  onClear: (id: number) => void;
  onVolumeChange: (id: number, volume: number) => void;
  onModeChange: (id: number) => void;
  progress: number;
  loading: boolean;
}) => {
  const hasAudio = pad.url !== null;
  const needsRelink = !!pad.needsRelink && !hasAudio;
  const hasAssignment = hasAudio || needsRelink;
  
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
    if (hasAssignment) {
      onClear(pad.id);
    }
  }, [hasAssignment, pad.id, onClear]);
  
  const modeLabel = pad.mode === 'oneshot' ? '1×' : pad.mode === 'loop' ? '∞' : '⏏';
  
  return (
    <div className="flex flex-col items-center gap-0.5">
      {/* Pad button */}
      <button
        className={`relative overflow-hidden w-14 h-12 rounded-md border-2 transition-all duration-75 flex flex-col items-center justify-center text-[10px] font-bold select-none ${
          pad.isPlaying
            ? 'shadow-lg scale-[0.97]'
            : hasAssignment
              ? 'hover:brightness-125 active:scale-95'
              : 'border-dashed opacity-50 hover:opacity-75'
        }`}
        style={{
          borderColor: hasAssignment ? pad.color : '#555',
          backgroundColor: pad.isPlaying
            ? `${pad.color}40`
            : hasAssignment
              ? `${pad.color}15`
              : 'transparent',
          boxShadow: pad.isPlaying ? `0 0 12px ${pad.color}60` : 'none',
        }}
        onClick={pad.mode === 'gate' ? undefined : handleClick}
        onMouseDown={pad.mode === 'gate' ? handleMouseDown : undefined}
        onMouseUp={pad.mode === 'gate' ? handleMouseUp : undefined}
        onMouseLeave={pad.mode === 'gate' && pad.isPlaying ? () => onStop(pad.id) : undefined}
        onContextMenu={handleContextMenu}
        title={needsRelink ? `${pad.name} needs relink (right-click to clear)` : hasAudio ? `${pad.name} (right-click to clear)` : 'Click to load sample'}
      >
        {hasAudio && (
          <div
            className='absolute left-0 bottom-0 h-1 pointer-events-none transition-[width] duration-75'
            style={{
              width: `${Math.round(progress * 100)}%`,
              backgroundColor: pad.color,
              opacity: pad.isPlaying ? 0.95 : 0.35,
            }}
          />
        )}
        <span 
          className="relative z-10 truncate w-full text-center px-1"
          style={{ color: hasAssignment ? pad.color : '#666' }}
        >
          {loading ? '...' : hasAssignment ? pad.name : '+'}
        </span>
        {hasAssignment && (
          <span className={`relative z-10 text-[9px] mt-0.5 ${needsRelink ? 'text-amber-400' : pad.isPlaying ? 'text-white' : 'text-neutral-500'}`}>
            {needsRelink ? 'RELINK' : pad.isPlaying ? pad.mode.toUpperCase() : pad.id + 1}
          </span>
        )}
      </button>
      
      {/* Controls row: mode + volume */}
      {hasAssignment && (
        <div className="flex items-center gap-1">
          <button
            className="text-[10px] px-1 py-0.5 rounded bg-neutral-800 hover:bg-neutral-700 text-neutral-400 disabled:opacity-50 disabled:cursor-not-allowed"
            onClick={() => onModeChange(pad.id)}
            disabled={needsRelink}
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
            disabled={needsRelink}
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
  const restoreSamplerPadMetadata = useStore(state => state.restoreSamplerPadMetadata);
  const clearSamplerPad = useStore(state => state.clearSamplerPad);
  const setSamplerPadVolume = useStore(state => state.setSamplerPadVolume);
  const setSamplerPadMode = useStore(state => state.setSamplerPadMode);
  const setSamplerPadPlaying = useStore(state => state.setSamplerPadPlaying);
  
  const fileInputRef = useRef<HTMLInputElement>(null);
  const pendingPadId = useRef<number>(0);
  const [loadingPadId, setLoadingPadId] = useState<number | null>(null);
  const [padProgress, setPadProgress] = useState<number[]>(() => new Array(8).fill(0));
  
  const engine = getDJSamplerEngine();

  useEffect(() => {
    if (typeof window === 'undefined') return;
    try {
      const stored = window.localStorage.getItem(SAMPLER_METADATA_STORAGE_KEY);
      if (!stored) return;
      const pads = JSON.parse(stored) as PersistedSamplerPad[];
      pads.forEach(pad => {
        if (pad.id >= 0 && pad.id < 8 && pad.name) {
          restoreSamplerPadMetadata(pad.id, {
            name: pad.name,
            volume: pad.volume,
            mode: pad.mode,
            color: pad.color,
          });
        }
      });
    } catch {
      // Ignore corrupted sampler metadata.
    }
  }, [restoreSamplerPadMetadata]);

  useEffect(() => {
    if (typeof window === 'undefined') return;
    const metadata = djSampler
      .filter(pad => pad.url || pad.needsRelink)
      .map<PersistedSamplerPad>(pad => ({
        id: pad.id,
        name: pad.name,
        volume: pad.volume,
        mode: pad.mode,
        color: pad.color,
      }));
    window.localStorage.setItem(SAMPLER_METADATA_STORAGE_KEY, JSON.stringify(metadata));
  }, [djSampler]);

  useEffect(() => {
    if (!djSampler.some(pad => pad.isPlaying)) {
      setPadProgress(new Array(8).fill(0));
      return;
    }

    let frame = 0;
    const update = () => {
      setPadProgress(djSampler.map(pad => pad.isPlaying ? engine.getProgress(pad.id) : 0));
      frame = requestAnimationFrame(update);
    };
    frame = requestAnimationFrame(update);
    return () => cancelAnimationFrame(frame);
  }, [djSampler, engine]);
  
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
    const currentUrl = djSampler[padId]?.url;
    engine.clearSample(padId);
    if (currentUrl?.startsWith('blob:')) URL.revokeObjectURL(currentUrl);
    clearSamplerPad(padId);
  }, [djSampler, engine, clearSamplerPad]);
  
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
    const previousUrl = djSampler[padId]?.url;
    
    setLoadingPadId(padId);
    const loaded = await engine.loadSample(padId, url);
    if (loaded) {
      if (previousUrl?.startsWith('blob:')) URL.revokeObjectURL(previousUrl);
      loadSamplerPad(padId, name, url);
    } else {
      URL.revokeObjectURL(url);
    }
    setLoadingPadId(null);
    
    // Reset input to allow re-selecting same file
    e.target.value = '';
  }, [djSampler, engine, loadSamplerPad]);
  
  return (
    <div className="flex flex-col gap-1">
      <div className="flex items-center justify-between px-1">
        <div className="text-[10px] text-neutral-500 font-bold uppercase tracking-wider">
          Sampler
        </div>
        <span
          className="text-[8px] text-amber-400/80 font-bold uppercase tracking-wider border border-amber-500/20 rounded px-1"
          title="Sampler metadata persists; local audio files must be relinked after reload."
        >
          Session
        </span>
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
            progress={padProgress[pad.id] || 0}
            loading={loadingPadId === pad.id}
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
