/**
 * ViiB MediaHub - Equalizer Panel Component
 * 
 * Floating 10-band parametric equalizer with preset support.
 * 
 * Features:
 * - 10 frequency bands (32Hz - 16kHz)
 * - Adjustable gain per band (-12dB to +12dB)
 * - Multiple presets (Flat, Rock, Jazz, Pop, etc.)
 * - Enable/disable toggle
 * - Reset to flat button
 * - Smooth value transitions
 * 
 * Integrates with AudioEngine for real-time EQ adjustment.
 * 
 * @module Equalizer
 */

import React from 'react';
import { useStore } from '../store';
import { EQ_FREQUENCIES, EQ_PRESETS } from '../utils';
import { X, Power, ChevronDown, RotateCcw } from 'lucide-react';
import { useFocusTrap } from '../hooks/useFocusTrap';
import { VIIB } from './ui/tokens';

export const EqualizerPanel: React.FC = () => {
  const { isEqOpen, toggleEqPanel, audioSettings, setEqBand, setEqEnabled, setEqPreset } = useStore();
  const dialogRef = useFocusTrap<HTMLDivElement>(isEqOpen, toggleEqPanel);

  if (!isEqOpen) return null;

  const handleReset = () => {
    setEqPreset('flat');
  };

  return (
        <div
          className={`fixed inset-0 ${VIIB.zIndex.popover} flex items-center justify-center p-4 bg-black/50 backdrop-blur-sm animate-in fade-in duration-200 motion-reduce:animate-none motion-reduce:transition-none`}
          onClick={toggleEqPanel}
        >
        <div
          ref={dialogRef}
          role="dialog"
          aria-modal="true"
          aria-labelledby="equalizer-dialog-title"
          tabIndex={-1}
          onClick={(e) => e.stopPropagation()}
          className="w-full max-w-4xl bg-surface-1 border border-surface-border rounded-2xl shadow-2xl flex flex-col overflow-hidden max-h-[90vh] outline-none"
        >
            
            {/* Header */}
            <div className="h-16 border-b border-surface-3 bg-surface-2 px-6 flex items-center justify-between flex-shrink-0">
                <div className="flex items-center gap-6">
                    <h2 id="equalizer-dialog-title" className="text-lg font-bold text-text-main tracking-wide">Equalizer</h2>
                    
                    {/* Toggle Switch */}
                    <button 
                        onClick={() => setEqEnabled(!audioSettings.eqEnabled)}
                        className={`flex items-center gap-2 px-4 py-1.5 rounded-full text-xs font-bold uppercase transition-all duration-300 ${
                            audioSettings.eqEnabled 
                                ? 'bg-brand text-black shadow-[0_0_15px_rgba(34,197,94,0.4)]' 
                                : 'bg-surface-border text-text-subtle hover:bg-surface-slider'
                        }`}
                    >
                        <Power size={14} strokeWidth={3} />
                        {audioSettings.eqEnabled ? 'On' : 'Off'}
                    </button>
                </div>

                <div className="flex items-center gap-4">
                    <button 
                        onClick={handleReset}
                        className="p-2 text-text-subtle hover:text-text-main transition-colors rounded-full hover:bg-surface-3"
                        title="Reset to Flat"
                    >
                        <RotateCcw size={18} />
                    </button>
                    <button
                        onClick={toggleEqPanel}
                        aria-label="Close equalizer"
                        className="p-2 text-text-subtle hover:text-text-main transition-colors rounded-full hover:bg-surface-3"
                    >
                        <X size={20} aria-hidden="true" />
                    </button>
                </div>
            </div>

            {/* Controls */}
            <div className="p-8 bg-surface-1 flex-1 flex flex-col items-center overflow-y-auto">
                
                {/* Preset Selector */}
                <div className="mb-10 relative z-20">
                    <div className="relative group min-w-[240px]">
                        <select 
                            value={audioSettings.activePresetId}
                            onChange={(e) => setEqPreset(e.target.value)}
                            className="w-full appearance-none bg-surface-3 hover:bg-surface-hover text-text-main py-3 pl-6 pr-10 rounded-full border border-surface-border hover:border-surface-slider outline-none cursor-pointer font-medium transition-all focus:border-brand focus:ring-1 focus:ring-accent-green/50 shadow-lg text-center"
                        >
                            <option value="custom">Manual</option>
                            <optgroup label="Presets">
                                {EQ_PRESETS.map(p => (
                                    <option key={p.id} value={p.id}>{p.name}</option>
                                ))}
                            </optgroup>
                        </select>
                        <ChevronDown size={16} className="absolute right-4 top-1/2 -translate-y-1/2 text-text-subtle pointer-events-none group-hover:text-text-main transition-colors" />
                    </div>
                </div>

                {/* Frequency Graph / Sliders */}
                <div className="w-full max-w-3xl overflow-x-auto pt-10 pb-2">
                    <div className="min-w-[440px] flex flex-col select-none">
                        <div className="flex items-stretch gap-2 h-56">
                            {/* Left dB Scale Column */}
                            <div className="w-7 shrink-0 flex flex-col justify-between items-end pr-1 py-2 text-[10px] font-mono text-text-secondary" aria-hidden="true">
                                <span className="-translate-y-1/2">+12</span>
                                <span>0</span>
                                <span className="translate-y-1/2">-12</span>
                            </div>

                            {/* Track Grid + Sliders */}
                            <div className="flex-1 flex justify-between gap-2 md:gap-4 relative">
                                {/* Background Grid Lines */}
                                <div className="absolute inset-x-0 top-2 bottom-2 pointer-events-none z-0">
                                    <div className="h-full w-full border-t border-b border-surface-border/30 relative">
                                        {/* +6 dB */}
                                        <div className="absolute top-[25%] left-0 w-full h-[1px] border-t border-dashed border-surface-border/20"></div>
                                        {/* 0 dB (Center) */}
                                        <div className="absolute top-[50%] left-0 w-full h-[1px] bg-brand/20 shadow-[0_0_10px_rgba(34,197,94,0.1)]"></div>
                                        {/* -6 dB */}
                                        <div className="absolute top-[75%] left-0 w-full h-[1px] border-t border-dashed border-surface-border/20"></div>
                                    </div>
                                </div>

                                {/* Sliders */}
                                {EQ_FREQUENCIES.map((freq, idx) => {
                                    const gain = audioSettings.eqBands[idx];
                                    const displayFreq = freq >= 1000 ? `${freq/1000}k` : freq;

                                    return (
                                        <div key={freq} className="relative z-10 flex-1 flex flex-col items-center group h-full py-2">
                                            {/* Gain Value Tooltip (Hover) */}
                                            <div
                                                className="absolute -top-9 opacity-0 group-hover:opacity-100 transition-opacity bg-surface-3 text-text-main text-[10px] font-mono py-1 px-2 rounded border border-surface-border shadow-lg whitespace-nowrap pointer-events-none transform translate-y-2 group-hover:translate-y-0 duration-200 z-40"
                                            >
                                                {gain > 0 ? '+' : ''}{gain.toFixed(1)} dB
                                            </div>

                                            {/* Slider Track Container */}
                                            <div className="flex-1 w-full flex items-center justify-center relative">
                                                {/* Vertical Track Visual */}
                                                <div className="absolute w-1 h-full bg-surface-highlight rounded-full overflow-hidden pointer-events-none">
                                                    <div
                                                        className="absolute w-full bg-surface-border left-0"
                                                        style={{
                                                            top: '50%',
                                                            height: '50%',
                                                            transformOrigin: 'top',
                                                        }}
                                                    />
                                                </div>

                                                {/* Active Fill Line */}
                                                <div
                                                    className={`absolute w-1 rounded-full transition-colors duration-150 pointer-events-none ${audioSettings.eqEnabled ? 'bg-brand/50' : 'bg-surface-slider/70'}`}
                                                    style={{
                                                        height: `${Math.abs(gain) / 12 * 50}%`,
                                                        top: gain > 0 ? '50%' : undefined,
                                                        bottom: gain <= 0 ? '50%' : undefined,
                                                        transform: gain > 0 ? 'translateY(-100%)' : 'none'
                                                    }}
                                                />

                                                {/* Custom Thumb Visual (Positioned by JS) */}
                                                <div
                                                    className={`pointer-events-none absolute left-1/2 -translate-x-1/2 -translate-y-1/2 w-4 h-4 rounded-full border-2 shadow-lg transition-all duration-75 z-20 ${
                                                        audioSettings.eqEnabled
                                                            ? 'bg-text-main border-brand group-hover:scale-125'
                                                            : 'bg-text-secondary border-surface-slider'
                                                    }`}
                                                    style={{
                                                        // Map -12..12 to 100%..0% height
                                                        top: `${((12 - gain) / 24) * 100}%`
                                                    }}
                                                >
                                                    <div className="absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 w-1 h-1 bg-black rounded-full opacity-20"></div>
                                                </div>

                                                {/* Actual Range Input (Rotated) */}
                                                <div className="absolute inset-0 flex items-center justify-center z-30 pointer-events-auto">
                                                    <input
                                                        type="range"
                                                        min="-12"
                                                        max="12"
                                                        step="0.5"
                                                        value={gain}
                                                        onChange={(e) => setEqBand(idx, parseFloat(e.target.value))}
                                                        className="w-[208px] h-12 opacity-0 cursor-pointer"
                                                        style={{ transform: 'rotate(-90deg)' }}
                                                        disabled={!audioSettings.eqEnabled}
                                                        aria-label={`${displayFreq}Hz gain`}
                                                    />
                                                </div>
                                            </div>
                                        </div>
                                    );
                                })}
                            </div>
                        </div>

                        {/* Frequency Labels Row */}
                        <div className="flex items-center gap-2 mt-3">
                            <div className="w-7 shrink-0" aria-hidden="true" />
                            <div className="flex-1 flex justify-between gap-2 md:gap-4">
                                {EQ_FREQUENCIES.map((freq) => {
                                    const displayFreq = freq >= 1000 ? `${freq/1000}k` : freq;
                                    return (
                                        <div key={freq} className="flex-1 text-center text-[11px] font-medium text-text-secondary font-mono tracking-tighter">
                                            {displayFreq}
                                        </div>
                                    );
                                })}
                            </div>
                        </div>
                    </div>
                </div>
                
                <div className="mt-6 text-xs text-text-secondary text-center">
                    Adjust frequencies to customize your sound experience.
                </div>
            </div>
        </div>
    </div>
  );
};