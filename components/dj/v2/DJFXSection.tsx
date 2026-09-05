/**
 * ViiB MediaHub - DJ FX Section Component (v2)
 * 
 * Horizontal FX strip with compact per-unit controls.
 * Provides Filter, Delay, Reverb, and Flanger per deck.
 * Styled to match professional DJ software FX strips.
 * 
 * Layout: [DECK A: Filter | Delay | Reverb | Flanger] --- [DECK B: Filter | Delay | Reverb | Flanger]
 * 
 * @module components/dj/v2/DJFXSection
 */

import React, { useCallback, useState, memo } from 'react';
import { useStore } from '../../../store';
import { useDJAudioEngineActions } from '../../../hooks/useDJAudioEngine';
import { DJEQKnob } from './DJEQKnob';
import { DJFXPad } from './DJFXPad';
import { DJBeatFXPanel } from './DJBeatFXPanel';
import type { DeckId, EffectType, DJLayoutMode } from '../../../slices/djMixerSlice';
import { ChevronDown, ChevronUp } from 'lucide-react';

// ============================================================================
// FX Unit for a single effect on a single deck
// ============================================================================

interface FXUnitProps {
  deck: DeckId;
  type: EffectType;
  label: string;
  color: string;
  enabledColor: string;
  compact?: boolean;
  expanded?: boolean;
}

const FX_CONFIGS = {
  filter: { label: 'FILTER', color: '#3b82f6', enabledColor: '#60a5fa' },
  delay: { label: 'DELAY', color: '#22c55e', enabledColor: '#4ade80' },
  reverb: { label: 'REVERB', color: '#a855f7', enabledColor: '#c084fc' },
  flanger: { label: 'FLANGER', color: '#f97316', enabledColor: '#fb923c' },
} as const;

const FXUnit = memo<FXUnitProps>(({ deck, type, label, color, enabledColor, compact = false, expanded = false }) => {
  const fx = useStore(state => {
    const deckState = deck === 'A' ? state.djDeckA : state.djDeckB;
    return deckState.fx[type];
  });

  // Unified action callbacks — each does store + engine in a single write
  const { setFilterFX, setDelayFX, setFlangerFX, setReverbFX } = useDJAudioEngineActions();

  const isEnabled = fx.enabled;

  // Toggle enable/disable — single call writes store + engine
  const handleToggle = useCallback(() => {
    const newEnabled = !fx.enabled;
    const f = fx as any;
    switch (type) {
      case 'filter':
        setFilterFX(deck, newEnabled, f.type || 'lowpass', f.frequency || 1000, f.resonance || 5);
        break;
      case 'delay':
        setDelayFX(deck, newEnabled, f.time || 0.25, f.feedback || 0.3, f.mix || 0.5);
        break;
      case 'reverb':
        setReverbFX(deck, newEnabled, f.roomSize || 0.5, f.damping || 0.5, f.mix || 0.3);
        break;
      case 'flanger':
        setFlangerFX(deck, newEnabled, f.rate || 0.5, f.depth || 0.5, f.feedback || 0.3);
        break;
    }
  }, [deck, type, fx, setFilterFX, setDelayFX, setReverbFX, setFlangerFX]);

  // Parameter change handlers — single call per change (store + engine)
  const handleParam1Change = useCallback((value: number) => {
    const f = fx as any;
    switch (type) {
      case 'filter':
        setFilterFX(deck, f.enabled, f.type, value, f.resonance);
        break;
      case 'delay':
        setDelayFX(deck, f.enabled, value, f.feedback, f.mix);
        break;
      case 'reverb':
        setReverbFX(deck, f.enabled, value, f.damping, f.mix);
        break;
      case 'flanger':
        setFlangerFX(deck, f.enabled, value, f.depth, f.feedback);
        break;
    }
  }, [deck, type, fx, setFilterFX, setDelayFX, setReverbFX, setFlangerFX]);

  const handleParam2Change = useCallback((value: number) => {
    const f = fx as any;
    switch (type) {
      case 'filter':
        setFilterFX(deck, f.enabled, f.type, f.frequency, value);
        break;
      case 'delay':
        setDelayFX(deck, f.enabled, f.time, value, f.mix);
        break;
      case 'reverb':
        setReverbFX(deck, f.enabled, f.roomSize, value, f.mix);
        break;
      case 'flanger':
        setFlangerFX(deck, f.enabled, f.rate, value, f.feedback);
        break;
    }
  }, [deck, type, fx, setFilterFX, setDelayFX, setReverbFX, setFlangerFX]);

  const handleDryWetChange = useCallback((value: number) => {
    const f = fx as any;
    switch (type) {
      case 'filter':
        // Filter doesn't have mix
        break;
      case 'delay':
        setDelayFX(deck, f.enabled, f.time, f.feedback, value);
        break;
      case 'reverb':
        setReverbFX(deck, f.enabled, f.roomSize, f.damping, value);
        break;
      case 'flanger':
        setFlangerFX(deck, f.enabled, f.rate, f.depth, value);
        break;
    }
  }, [deck, type, fx, setDelayFX, setReverbFX, setFlangerFX]);

  // Get current param values based on FX type
  const getParams = () => {
    const f = fx as any;
    switch (type) {
      case 'filter':
        return {
          param1: { label: 'FREQ', value: f.frequency || 1000, min: -24, max: 12 },
          param2: { label: 'RES', value: f.resonance || 5, min: -24, max: 12 },
          hasWet: false,
          wet: { label: '', value: 0, min: 0, max: 1 },
        };
      case 'delay':
        return {
          param1: { label: 'TIME', value: mapToKnobRange(f.time || 0.25, 0.01, 1), min: -24, max: 12 },
          param2: { label: 'FDBK', value: mapToKnobRange(f.feedback || 0.3, 0, 0.9), min: -24, max: 12 },
          hasWet: true,
          wet: { label: 'MIX', value: mapToKnobRange(f.mix || 0.5, 0, 1), min: -24, max: 12 },
        };
      case 'reverb':
        return {
          param1: { label: 'ROOM', value: mapToKnobRange(f.roomSize || 0.5, 0.1, 1), min: -24, max: 12 },
          param2: { label: 'DAMP', value: mapToKnobRange(f.damping || 0.5, 0, 1), min: -24, max: 12 },
          hasWet: true,
          wet: { label: 'MIX', value: mapToKnobRange(f.mix || 0.3, 0, 1), min: -24, max: 12 },
        };
      case 'flanger':
        return {
          param1: { label: 'RATE', value: mapToKnobRange(f.rate || 0.5, 0.1, 5), min: -24, max: 12 },
          param2: { label: 'DPTH', value: mapToKnobRange(f.depth || 0.5, 0, 1), min: -24, max: 12 },
          hasWet: true,
          wet: { label: 'FDBK', value: mapToKnobRange(f.feedback || 0.3, 0, 0.9), min: -24, max: 12 },
        };
      default:
        return {
          param1: { label: 'P1', value: 0, min: -24, max: 12 },
          param2: { label: 'P2', value: 0, min: -24, max: 12 },
          hasWet: false,
          wet: { label: '', value: 0, min: 0, max: 1 },
        };
    }
  };

  const params = getParams();

  // Wrap param changes to convert from knob range (-24 to +12) back to real values
  const handleKnobParam1 = useCallback((knobValue: number) => {
    const f = fx as any;
    switch (type) {
      case 'filter': {
        // Frequency: map knob (-24 to 12) → (20 to 20000) logarithmic
        const normalized = (knobValue + 24) / 36;
        const freq = 20 * Math.pow(1000, normalized);
        handleParam1Change(Math.round(freq));
        break;
      }
      case 'delay':
        handleParam1Change(knobToReal(knobValue, 0.01, 1));
        break;
      case 'reverb':
        handleParam1Change(knobToReal(knobValue, 0.1, 1));
        break;
      case 'flanger':
        handleParam1Change(knobToReal(knobValue, 0.1, 5));
        break;
    }
  }, [type, handleParam1Change, fx]);

  const handleKnobParam2 = useCallback((knobValue: number) => {
    switch (type) {
      case 'filter': {
        const normalized = (knobValue + 24) / 36;
        const res = 0.1 + normalized * 19.9;
        handleParam2Change(Number(res.toFixed(1)));
        break;
      }
      case 'delay':
        handleParam2Change(knobToReal(knobValue, 0, 0.9));
        break;
      case 'reverb':
        handleParam2Change(knobToReal(knobValue, 0, 1));
        break;
      case 'flanger':
        handleParam2Change(knobToReal(knobValue, 0, 1));
        break;
    }
  }, [type, handleParam2Change]);

  const handleKnobWet = useCallback((knobValue: number) => {
    switch (type) {
      case 'delay':
        handleDryWetChange(knobToReal(knobValue, 0, 1));
        break;
      case 'reverb':
        handleDryWetChange(knobToReal(knobValue, 0, 1));
        break;
      case 'flanger':
        handleDryWetChange(knobToReal(knobValue, 0, 0.9));
        break;
    }
  }, [type, handleDryWetChange]);

  const knobSize = expanded ? 44 : 32;

  // Compact mode: toggle tab + always-rendered macro knob (greyed when off,
  // so toggling does NOT shift neighbour layout — see review §2.6).
  // MIX knob uses size=32 — paired inline with the 36-px toggle to keep the
  // FX strip from ballooning and squeezing the deck height.
  if (compact) {
    return (
      <div className={`flex items-center gap-1.5 px-1.5 py-0.5 rounded transition-all duration-150 ${
        isEnabled ? 'bg-[#1e1e1e]' : 'bg-transparent'
      }`}>
        <button
          onClick={handleToggle}
          aria-pressed={isEnabled}
          className={`
            flex items-center gap-1 px-2.5 py-1 rounded text-[11px] font-bold uppercase tracking-wider
            transition-all duration-100 border min-w-[64px] justify-center min-h-[36px]
            ${isEnabled
              ? 'text-white border-current/30'
              : 'bg-[#222] text-neutral-600 border-[#333] hover:text-neutral-400 hover:border-[#444]'}
          `}
          style={isEnabled ? {
            backgroundColor: color,
            borderColor: `${color}60`,
            boxShadow: `0 0 6px ${color}30`,
          } : undefined}
        >
          {isEnabled && <span className="w-1.5 h-1.5 rounded-full bg-white/80 flex-shrink-0" />}
          {label}
        </button>
        {params.hasWet && (
          <DJEQKnob
            label={params.wet.label}
            value={params.wet.value}
            onChange={handleKnobWet}
            color={isEnabled ? enabledColor : '#444'}
            size={32}
          />
        )}
      </div>
    );
  }

  return (
    <div className={`flex flex-col items-center gap-1 px-1.5 py-1 rounded-md transition-all duration-150 ${
      isEnabled ? 'bg-[#1e1e1e] border border-current/20' : 'bg-[#141414] border border-transparent'
    }`}
      style={isEnabled ? { borderColor: `${color}30` } : undefined}
    >
      {/* Enable/Disable button */}
      <button
        onClick={handleToggle}
        aria-pressed={isEnabled}
        className={`
          w-full px-2 py-1.5 rounded text-[11px] font-bold uppercase tracking-wider
          transition-all duration-100 border flex items-center justify-center gap-1 min-h-[36px]
          ${isEnabled
            ? 'text-white border-transparent'
            : 'bg-[#222] text-neutral-600 border-[#333] hover:text-neutral-400 hover:border-[#444]'}
        `}
        style={isEnabled ? {
          backgroundColor: color,
          boxShadow: `0 0 8px ${color}40`,
        } : undefined}
      >
        {isEnabled && <span className="w-1.5 h-1.5 rounded-full bg-white/80 flex-shrink-0" />}
        {label}
      </button>

      {/* Knobs row */}
      <div className={`flex ${expanded ? 'gap-1.5' : 'gap-0.5'}`}>
        <DJEQKnob
          label={params.param1.label}
          value={params.param1.value}
          onChange={handleKnobParam1}
          color={isEnabled ? enabledColor : '#555'}
          size={knobSize}
        />
        <DJEQKnob
          label={params.param2.label}
          value={params.param2.value}
          onChange={handleKnobParam2}
          color={isEnabled ? enabledColor : '#555'}
          size={knobSize}
        />
        {params.hasWet && (
          <DJEQKnob
            label={params.wet.label}
            value={params.wet.value}
            onChange={handleKnobWet}
            color={isEnabled ? enabledColor : '#555'}
            size={knobSize}
          />
        )}
      </div>
    </div>
  );
});

FXUnit.displayName = 'FXUnit';

// ============================================================================
// Helper functions for knob ↔ real value mapping
// ============================================================================

/** Map a real value (min..max) to the EQ knob range (-24 to +12) */
function mapToKnobRange(value: number, min: number, max: number): number {
  const normalized = (value - min) / (max - min);
  return -24 + normalized * 36;
}

/** Map knob range (-24 to +12) back to real value (min..max) */
function knobToReal(knobValue: number, min: number, max: number): number {
  const normalized = (knobValue + 24) / 36;
  return min + normalized * (max - min);
}

// ============================================================================
// Main FX Section Component
// ============================================================================

interface DJFXSectionProps {
  className?: string;
}

export const DJFXSection: React.FC<DJFXSectionProps> = ({ className = '' }) => {
  const [collapsed, setCollapsed] = useState(false);

  // Layout mode awareness
  const layoutMode = useStore(s => s.djMixer?.djLayoutMode || 'perf') as DJLayoutMode;
  const isCompact = layoutMode === 'perf' || layoutMode === 'browse';
  const isExpanded = layoutMode === 'fx';

  // Count active effects per deck
  const activeA = useStore(state =>
    Object.values(state.djDeckA.fx).filter(fx => fx.enabled).length);
  const activeB = useStore(state =>
    Object.values(state.djDeckB.fx).filter(fx => fx.enabled).length);

  return (
    <div className={`bg-[#161616] border-b border-[#2a2a2a] ${className}`}>
      {/* Header */}
      <div
        role="button" tabIndex={0} aria-label="Toggle effects" aria-expanded={!collapsed}
        onKeyDown={event => { if (event.key === 'Enter' || event.key === ' ') { event.preventDefault(); setCollapsed(value => !value); } }}
        className="flex items-center justify-between px-4 h-6 cursor-pointer select-none hover:bg-white/5 transition-colors"
        onClick={() => setCollapsed(!collapsed)}
      >
        <div className="flex items-center gap-3">
          <span className="text-[10px] font-bold text-neutral-500 uppercase tracking-widest">FX</span>
          {activeA > 0 && (
            <span className="text-[10px] font-bold text-blue-400 bg-blue-500/15 px-1.5 py-0.5 rounded">
              A: {activeA}
            </span>
          )}
          {activeB > 0 && (
            <span className="text-[10px] font-bold text-purple-400 bg-purple-500/15 px-1.5 py-0.5 rounded">
              B: {activeB}
            </span>
          )}
        </div>
        {collapsed ? (
          <ChevronDown size={12} className="text-neutral-500" />
        ) : (
          <ChevronUp size={12} className="text-neutral-500" />
        )}
      </div>

      {/* FX Strip — Deck A FX | X-Y FX Pad (centre, fills the dead space) | Deck B FX */}
      {!collapsed && (
        <div className={`flex items-stretch px-2 pb-2 gap-2 ${isExpanded ? 'pt-1' : ''}`}>
          {/* Deck A FX */}
          <div className={`flex-1 flex ${isCompact ? 'items-center' : ''} gap-1 bg-[#111] rounded-md p-1.5 border border-[#222]`}>
            <div className="text-[10px] font-bold text-blue-400 writing-vertical flex items-center justify-center w-3 mr-0.5"
                 style={{ writingMode: 'vertical-rl', textOrientation: 'mixed' }}>
              A
            </div>
            {(Object.keys(FX_CONFIGS) as EffectType[]).map(fxType => (
              <FXUnit
                key={`A-${fxType}`}
                deck="A"
                type={fxType}
                label={FX_CONFIGS[fxType].label}
                color={FX_CONFIGS[fxType].color}
                enabledColor={FX_CONFIGS[fxType].enabledColor}
                compact={isCompact}
                expanded={isExpanded}
              />
            ))}
          </div>

          {/* X-Y FX Pad — fills the centre gap that was previously empty.
              Drag morphs filter cutoff (X) + resonance (Y) for the selected deck(s);
              releasing the pad smoothly returns to neutral. */}
          <div className={`flex-shrink-0 flex items-center justify-center bg-[#0e0e0e] rounded-md border border-[#222] px-2 py-1 ${isExpanded ? 'gap-2' : ''}`}>
            {isExpanded && <DJBeatFXPanel />}
            <DJFXPad size={isExpanded ? 150 : 96} />
          </div>

          {/* Deck B FX */}
          <div className={`flex-1 flex ${isCompact ? 'items-center' : ''} gap-1 bg-[#111] rounded-md p-1.5 border border-[#222]`}>
            {(Object.keys(FX_CONFIGS) as EffectType[]).map(fxType => (
              <FXUnit
                key={`B-${fxType}`}
                deck="B"
                type={fxType}
                label={FX_CONFIGS[fxType].label}
                color={FX_CONFIGS[fxType].color}
                enabledColor={FX_CONFIGS[fxType].enabledColor}
                compact={isCompact}
                expanded={isExpanded}
              />
            ))}
            <div className="text-[10px] font-bold text-purple-400 writing-vertical flex items-center justify-center w-3 ml-0.5"
                 style={{ writingMode: 'vertical-rl', textOrientation: 'mixed' }}>
              B
            </div>
          </div>
        </div>
      )}
    </div>
  );
};

export default React.memo(DJFXSection);
