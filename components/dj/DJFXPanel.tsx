/**
 * ViiB MediaHub - DJ FX Panel Component
 * 
 * Provides controls for DJ effects per deck:
 * - Filter (Low-pass / High-pass sweep)
 * - Delay (Echo with feedback)
 * - Reverb (Room simulation)
 * - Flanger (LFO-modulated delay)
 * 
 * @module components/dj/DJFXPanel
 */

import React, { useCallback, useState } from 'react';
import { useStore } from '../../store';
import { useDJAudioEngine } from '../../hooks/useDJAudioEngine';
import { ChevronDown, ChevronUp } from 'lucide-react';
import type { DeckId, EffectType } from '../../slices/djMixerSlice';

interface DJFXPanelProps {
  deck: DeckId;
  defaultCollapsed?: boolean;
}

export const DJFXPanel: React.FC<DJFXPanelProps> = ({ deck, defaultCollapsed = false }) => {
  const [isCollapsed, setIsCollapsed] = useState(defaultCollapsed);
  // Only subscribe to fx state, not the entire deck state (avoids re-renders on position updates)
  const fx = useStore(state => deck === 'A' ? state.djDeckA.fx : state.djDeckB.fx);
  const { setFilterFX: storeSetFilterFX, setDelayFX: storeSetDelayFX, 
          setReverbFX: storeSetReverbFX, setFlangerFX: storeSetFlangerFX,
          toggleFX } = useStore();
  const { setFilterFX, setDelayFX, setFlangerFX, setReverbFX } = useDJAudioEngine();

  // Handle Filter FX toggle
  const handleFilterToggle = useCallback(() => {
    const newEnabled = !fx.filter.enabled;
    toggleFX(deck, 'filter');
    setFilterFX(deck, newEnabled, fx.filter.type, fx.filter.frequency, fx.filter.resonance);
  }, [deck, fx.filter, toggleFX, setFilterFX]);

  // Handle Filter type change
  const handleFilterTypeChange = useCallback((type: 'lowpass' | 'highpass') => {
    storeSetFilterFX(deck, { type });
    if (fx.filter.enabled) {
      setFilterFX(deck, true, type, fx.filter.frequency, fx.filter.resonance);
    }
  }, [deck, fx.filter, storeSetFilterFX, setFilterFX]);

  // Handle Filter frequency change
  const handleFilterFrequencyChange = useCallback((frequency: number) => {
    storeSetFilterFX(deck, { frequency });
    if (fx.filter.enabled) {
      setFilterFX(deck, true, fx.filter.type, frequency, fx.filter.resonance);
    }
  }, [deck, fx.filter, storeSetFilterFX, setFilterFX]);

  // Handle Filter resonance change
  const handleFilterResonanceChange = useCallback((resonance: number) => {
    storeSetFilterFX(deck, { resonance });
    if (fx.filter.enabled) {
      setFilterFX(deck, true, fx.filter.type, fx.filter.frequency, resonance);
    }
  }, [deck, fx.filter, storeSetFilterFX, setFilterFX]);

  // Handle Delay FX toggle
  const handleDelayToggle = useCallback(() => {
    const newEnabled = !fx.delay.enabled;
    toggleFX(deck, 'delay');
    setDelayFX(deck, newEnabled, fx.delay.time, fx.delay.feedback, fx.delay.mix);
  }, [deck, fx.delay, toggleFX, setDelayFX]);

  // Handle Delay parameter changes
  const handleDelayTimeChange = useCallback((time: number) => {
    storeSetDelayFX(deck, { time });
    if (fx.delay.enabled) {
      setDelayFX(deck, true, time, fx.delay.feedback, fx.delay.mix);
    }
  }, [deck, fx.delay, storeSetDelayFX, setDelayFX]);

  const handleDelayFeedbackChange = useCallback((feedback: number) => {
    storeSetDelayFX(deck, { feedback });
    if (fx.delay.enabled) {
      setDelayFX(deck, true, fx.delay.time, feedback, fx.delay.mix);
    }
  }, [deck, fx.delay, storeSetDelayFX, setDelayFX]);

  const handleDelayMixChange = useCallback((mix: number) => {
    storeSetDelayFX(deck, { mix });
    if (fx.delay.enabled) {
      setDelayFX(deck, true, fx.delay.time, fx.delay.feedback, mix);
    }
  }, [deck, fx.delay, storeSetDelayFX, setDelayFX]);

  // Handle Reverb FX toggle
  const handleReverbToggle = useCallback(() => {
    const newEnabled = !fx.reverb.enabled;
    toggleFX(deck, 'reverb');
    setReverbFX(deck, newEnabled, fx.reverb.roomSize, fx.reverb.damping, fx.reverb.mix);
  }, [deck, fx.reverb, toggleFX, setReverbFX]);

  // Handle Reverb parameter changes
  const handleReverbRoomChange = useCallback((roomSize: number) => {
    storeSetReverbFX(deck, { roomSize });
    if (fx.reverb.enabled) {
      setReverbFX(deck, true, roomSize, fx.reverb.damping, fx.reverb.mix);
    }
  }, [deck, fx.reverb, storeSetReverbFX, setReverbFX]);

  const handleReverbDampingChange = useCallback((damping: number) => {
    storeSetReverbFX(deck, { damping });
    if (fx.reverb.enabled) {
      setReverbFX(deck, true, fx.reverb.roomSize, damping, fx.reverb.mix);
    }
  }, [deck, fx.reverb, storeSetReverbFX, setReverbFX]);

  const handleReverbMixChange = useCallback((mix: number) => {
    storeSetReverbFX(deck, { mix });
    if (fx.reverb.enabled) {
      setReverbFX(deck, true, fx.reverb.roomSize, fx.reverb.damping, mix);
    }
  }, [deck, fx.reverb, storeSetReverbFX, setReverbFX]);

  // Handle Flanger FX toggle
  const handleFlangerToggle = useCallback(() => {
    const newEnabled = !fx.flanger.enabled;
    toggleFX(deck, 'flanger');
    setFlangerFX(deck, newEnabled, fx.flanger.rate, fx.flanger.depth, fx.flanger.feedback);
  }, [deck, fx.flanger, toggleFX, setFlangerFX]);

  // Handle Flanger parameter changes
  const handleFlangerRateChange = useCallback((rate: number) => {
    storeSetFlangerFX(deck, { rate });
    if (fx.flanger.enabled) {
      setFlangerFX(deck, true, rate, fx.flanger.depth, fx.flanger.feedback);
    }
  }, [deck, fx.flanger, storeSetFlangerFX, setFlangerFX]);

  const handleFlangerDepthChange = useCallback((depth: number) => {
    storeSetFlangerFX(deck, { depth });
    if (fx.flanger.enabled) {
      setFlangerFX(deck, true, fx.flanger.rate, depth, fx.flanger.feedback);
    }
  }, [deck, fx.flanger, storeSetFlangerFX, setFlangerFX]);

  const handleFlangerFeedbackChange = useCallback((feedback: number) => {
    storeSetFlangerFX(deck, { feedback });
    if (fx.flanger.enabled) {
      setFlangerFX(deck, true, fx.flanger.rate, fx.flanger.depth, feedback);
    }
  }, [deck, fx.flanger, storeSetFlangerFX, setFlangerFX]);

  // FX Button component
  const FXButton: React.FC<{ 
    label: string; 
    enabled: boolean; 
    onClick: () => void;
    color?: string;
  }> = ({ label, enabled, onClick, color = 'brand' }) => (
    <button
      onClick={onClick}
      className={`
        px-3 py-1.5 rounded text-xs font-bold uppercase tracking-wide
        transition-all duration-150
        ${enabled 
          ? `bg-${color} text-white shadow-lg shadow-${color}/30` 
          : 'bg-surface-2 text-neutral-400 hover:bg-surface-1'
        }
      `}
    >
      {label}
    </button>
  );

  // Knob/Slider component
  const FXKnob: React.FC<{
    label: string;
    value: number;
    min: number;
    max: number;
    step?: number;
    onChange: (value: number) => void;
    disabled?: boolean;
    format?: (v: number) => string;
  }> = ({ label, value, min, max, step = 0.01, onChange, disabled, format }) => (
    <div className="flex flex-col items-center gap-1">
      <span className="text-[10px] text-neutral-500 uppercase">{label}</span>
      <input
        type="range"
        min={min}
        max={max}
        step={step}
        value={value}
        onChange={(e) => onChange(parseFloat(e.target.value))}
        disabled={disabled}
        className="w-16 h-1 accent-brand disabled:opacity-30"
      />
      <span className="text-[10px] text-neutral-400">
        {format ? format(value) : value.toFixed(2)}
      </span>
    </div>
  );

  // Count active effects for collapsed view indicator
  const activeEffectsCount = [fx.filter.enabled, fx.delay.enabled, fx.reverb.enabled, fx.flanger.enabled].filter(Boolean).length;

  return (
    <div className="bg-surface-1 rounded-lg p-2">
      {/* Header with collapse toggle */}
      <div 
        className="flex items-center justify-between cursor-pointer select-none"
        onClick={() => setIsCollapsed(!isCollapsed)}
      >
        <h3 className="text-xs font-bold text-neutral-300 uppercase tracking-wider">
          FX - Deck {deck}
        </h3>
        <div className="flex items-center gap-2">
          {/* Active effects indicator (visible when collapsed) */}
          {activeEffectsCount > 0 && (
            <span className="text-[10px] font-bold text-brand bg-brand/20 px-1.5 py-0.5 rounded">
              {activeEffectsCount} ON
            </span>
          )}
          {isCollapsed ? (
            <ChevronDown size={14} className="text-neutral-400" />
          ) : (
            <ChevronUp size={14} className="text-neutral-400" />
          )}
        </div>
      </div>

      {/* Collapsible content */}
      {!isCollapsed && (
        <div className="grid grid-cols-2 gap-2 mt-2">
        {/* Filter FX */}
        <div className="bg-surface-0 rounded p-2">
          <div className="flex items-center justify-between mb-2">
            <span className="text-xs font-medium text-neutral-300">Filter</span>
            <FXButton 
              label={fx.filter.enabled ? 'ON' : 'OFF'} 
              enabled={fx.filter.enabled} 
              onClick={handleFilterToggle}
              color="blue-500"
            />
          </div>
          <div className="flex items-center gap-2 mb-2">
            <button
              onClick={() => handleFilterTypeChange('lowpass')}
              className={`px-2 py-0.5 text-[10px] rounded ${
                fx.filter.type === 'lowpass' ? 'bg-blue-500/30 text-blue-300' : 'bg-surface-2 text-neutral-500'
              }`}
            >
              LP
            </button>
            <button
              onClick={() => handleFilterTypeChange('highpass')}
              className={`px-2 py-0.5 text-[10px] rounded ${
                fx.filter.type === 'highpass' ? 'bg-blue-500/30 text-blue-300' : 'bg-surface-2 text-neutral-500'
              }`}
            >
              HP
            </button>
          </div>
          <div className="flex gap-2">
            <FXKnob
              label="Freq"
              value={fx.filter.frequency}
              min={20}
              max={20000}
              step={10}
              onChange={handleFilterFrequencyChange}
              disabled={!fx.filter.enabled}
              format={(v) => v >= 1000 ? `${(v/1000).toFixed(1)}k` : `${v}`}
            />
            <FXKnob
              label="Res"
              value={fx.filter.resonance}
              min={0.1}
              max={20}
              step={0.1}
              onChange={handleFilterResonanceChange}
              disabled={!fx.filter.enabled}
            />
          </div>
        </div>

        {/* Delay FX */}
        <div className="bg-surface-0 rounded p-2">
          <div className="flex items-center justify-between mb-2">
            <span className="text-xs font-medium text-neutral-300">Delay</span>
            <FXButton 
              label={fx.delay.enabled ? 'ON' : 'OFF'} 
              enabled={fx.delay.enabled} 
              onClick={handleDelayToggle}
              color="green-500"
            />
          </div>
          <div className="flex gap-2">
            <FXKnob
              label="Time"
              value={fx.delay.time}
              min={0.01}
              max={1}
              step={0.01}
              onChange={handleDelayTimeChange}
              disabled={!fx.delay.enabled}
              format={(v) => `${(v * 1000).toFixed(0)}ms`}
            />
            <FXKnob
              label="Fdbk"
              value={fx.delay.feedback}
              min={0}
              max={0.9}
              step={0.05}
              onChange={handleDelayFeedbackChange}
              disabled={!fx.delay.enabled}
            />
            <FXKnob
              label="Mix"
              value={fx.delay.mix}
              min={0}
              max={1}
              step={0.05}
              onChange={handleDelayMixChange}
              disabled={!fx.delay.enabled}
            />
          </div>
        </div>

        {/* Reverb FX */}
        <div className="bg-surface-0 rounded p-2">
          <div className="flex items-center justify-between mb-2">
            <span className="text-xs font-medium text-neutral-300">Reverb</span>
            <FXButton 
              label={fx.reverb.enabled ? 'ON' : 'OFF'} 
              enabled={fx.reverb.enabled} 
              onClick={handleReverbToggle}
              color="purple-500"
            />
          </div>
          <div className="flex gap-2">
            <FXKnob
              label="Room"
              value={fx.reverb.roomSize}
              min={0.1}
              max={1}
              step={0.05}
              onChange={handleReverbRoomChange}
              disabled={!fx.reverb.enabled}
            />
            <FXKnob
              label="Damp"
              value={fx.reverb.damping}
              min={0}
              max={1}
              step={0.05}
              onChange={handleReverbDampingChange}
              disabled={!fx.reverb.enabled}
            />
            <FXKnob
              label="Mix"
              value={fx.reverb.mix}
              min={0}
              max={1}
              step={0.05}
              onChange={handleReverbMixChange}
              disabled={!fx.reverb.enabled}
            />
          </div>
        </div>

        {/* Flanger FX */}
        <div className="bg-surface-0 rounded p-2">
          <div className="flex items-center justify-between mb-2">
            <span className="text-xs font-medium text-neutral-300">Flanger</span>
            <FXButton 
              label={fx.flanger.enabled ? 'ON' : 'OFF'} 
              enabled={fx.flanger.enabled} 
              onClick={handleFlangerToggle}
              color="orange-500"
            />
          </div>
          <div className="flex gap-2">
            <FXKnob
              label="Rate"
              value={fx.flanger.rate}
              min={0.1}
              max={5}
              step={0.1}
              onChange={handleFlangerRateChange}
              disabled={!fx.flanger.enabled}
              format={(v) => `${v.toFixed(1)}Hz`}
            />
            <FXKnob
              label="Depth"
              value={fx.flanger.depth}
              min={0}
              max={1}
              step={0.05}
              onChange={handleFlangerDepthChange}
              disabled={!fx.flanger.enabled}
            />
            <FXKnob
              label="Fdbk"
              value={fx.flanger.feedback}
              min={0}
              max={0.9}
              step={0.05}
              onChange={handleFlangerFeedbackChange}
              disabled={!fx.flanger.enabled}
            />
          </div>
        </div>
      </div>
      )}
    </div>
  );
};

export default DJFXPanel;
