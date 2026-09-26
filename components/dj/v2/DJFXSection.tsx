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
  /** Deck-local rack module: label toggle over 2–3 compact knobs (Plan §10A.12). */
  rack?: boolean;
}

export const FX_CONFIGS = {
  filter: { label: 'FILTER', color: '#3b82f6', enabledColor: '#60a5fa' },
  delay: { label: 'DELAY', color: '#22c55e', enabledColor: '#4ade80' },
  reverb: { label: 'REVERB', color: '#a855f7', enabledColor: '#c084fc' },
  flanger: { label: 'FLANGER', color: '#f97316', enabledColor: '#fb923c' },
} as const;

export const FXUnit = memo<FXUnitProps>(({ deck, type, label, color, enabledColor, compact = false, expanded = false, rack = false }) => {
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
        setFilterFX(deck, newEnabled, f.type || 'lowpass', f.frequency ?? 1000, f.resonance ?? 5);
        break;
      case 'delay':
        setDelayFX(deck, newEnabled, f.time ?? 0.25, f.feedback ?? 0.3, f.mix ?? 0.5);
        break;
      case 'reverb':
        setReverbFX(deck, newEnabled, f.roomSize ?? 0.5, f.damping ?? 0.5, f.mix ?? 0.3);
        break;
      case 'flanger':
        setFlangerFX(deck, newEnabled, f.rate ?? 0.5, f.depth ?? 0.5, f.feedback ?? 0.3);
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
          param1: { label: 'FREQ', value: mapToKnobRange(Math.log((f.frequency ?? 1000) / 20) / Math.log(1000), 0, 1), min: -24, max: 12 },
          param2: { label: 'RES', value: mapToKnobRange(f.resonance ?? 5, 0.1, 20), min: -24, max: 12 },
          hasWet: false,
          wet: { label: '', value: 0, min: 0, max: 1 },
        };
      case 'delay':
        return {
          param1: { label: 'TIME', value: mapToKnobRange(f.time ?? 0.25, 0.01, 1), min: -24, max: 12 },
          param2: { label: 'FDBK', value: mapToKnobRange(f.feedback ?? 0.3, 0, 0.9), min: -24, max: 12 },
          hasWet: true,
          wet: { label: 'MIX', value: mapToKnobRange(f.mix ?? 0.5, 0, 1), min: -24, max: 12 },
        };
      case 'reverb':
        return {
          param1: { label: 'ROOM', value: mapToKnobRange(f.roomSize ?? 0.5, 0.1, 1), min: -24, max: 12 },
          param2: { label: 'DAMP', value: mapToKnobRange(f.damping ?? 0.5, 0, 1), min: -24, max: 12 },
          hasWet: true,
          wet: { label: 'MIX', value: mapToKnobRange(f.mix ?? 0.3, 0, 1), min: -24, max: 12 },
        };
      case 'flanger':
        return {
          param1: { label: 'RATE', value: mapToKnobRange(f.rate ?? 0.5, 0.1, 5), min: -24, max: 12 },
          param2: { label: 'DPTH', value: mapToKnobRange(f.depth ?? 0.5, 0, 1), min: -24, max: 12 },
          hasWet: true,
          wet: { label: 'FDBK', value: mapToKnobRange(f.feedback ?? 0.3, 0, 0.9), min: -24, max: 12 },
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

  const knobSize = expanded ? 44 : compact ? 32 : 40;
  const actual = fx as any;
  const percent = (value: number) => `${Math.round(value * 100)}%`;
  const valueText1 = type === 'filter' ? `${Math.round(actual.frequency)} Hz`
    : type === 'delay' ? `${Math.round(actual.time * 1000)} ms`
    : type === 'flanger' ? `${actual.rate.toFixed(2)} Hz` : percent(actual.roomSize);
  const valueText2 = type === 'filter' ? `${actual.resonance.toFixed(1)} Q`
    : percent(type === 'delay' ? actual.feedback : type === 'reverb' ? actual.damping : actual.depth);
  const wetText = percent(type === 'flanger' ? actual.feedback : actual.mix);

  if (rack) {
    const knobColor = isEnabled ? `var(--dj-deck-${deck === 'A' ? 'a' : 'b'}-bright)` : 'var(--dj-text-muted)';
    return (
      <div className='dj-fx-module' data-enabled={isEnabled} data-fx={type}>
        <button type='button' className='dj-fx-module-toggle' onClick={handleToggle} aria-pressed={isEnabled}
          aria-label={`${label} on Deck ${deck}`} title={`${isEnabled ? 'Disable' : 'Enable'} ${label.toLowerCase()} on Deck ${deck}`}>
          {label}
        </button>
        <div className='dj-fx-knobs'>
          <DJEQKnob label={params.param1.label} ariaLabel={`${label.charAt(0) + label.slice(1).toLowerCase()} ${params.param1.label}, Deck ${deck}`} valueText={valueText1} value={params.param1.value} onChange={handleKnobParam1} color={knobColor} size={36} compact labelBelow className='dj-fx-knob' />
          <DJEQKnob label={params.param2.label} ariaLabel={`${label.charAt(0) + label.slice(1).toLowerCase()} ${params.param2.label}, Deck ${deck}`} valueText={valueText2} value={params.param2.value} onChange={handleKnobParam2} color={knobColor} size={36} compact labelBelow className='dj-fx-knob' />
          {params.hasWet && (
            <DJEQKnob label={params.wet.label} ariaLabel={`${label.charAt(0) + label.slice(1).toLowerCase()} ${params.wet.label}, Deck ${deck}`} valueText={wetText} value={params.wet.value} onChange={handleKnobWet} color={knobColor} size={36} compact labelBelow className='dj-fx-knob' />
          )}
        </div>
      </div>
    );
  }

  // Compact mode: toggle tab + always-rendered macro knob (greyed when off,
  // so toggling does NOT shift neighbour layout — see review §2.6).
  // MIX knob uses size=32 — paired inline with the 36-px toggle to keep the
  // FX strip from ballooning and squeezing the deck height.
  if (compact) {
    return (
      <div className={`flex items-center gap-1.5 px-1.5 py-0.5 rounded transition-all duration-150 ${
        isEnabled ? 'bg-[var(--dj-surface-3)]' : 'bg-transparent'
      }`}>
        <button
          onClick={handleToggle}
          aria-pressed={isEnabled}
          className={`
            flex items-center gap-1 px-2.5 py-1 rounded text-[12px] font-bold uppercase tracking-wider
            transition-all duration-100 border min-w-[64px] justify-center min-h-[36px]
            ${isEnabled
              ? 'text-white border-current/30'
              : 'bg-[var(--dj-surface-3)] text-[var(--dj-text-secondary)] border-[var(--dj-border-light)] hover:text-[var(--dj-text-secondary)] hover:border-[var(--dj-border-hover)]'}
          `}
          style={isEnabled ? {
            backgroundColor: color,
            borderColor: `color-mix(in srgb, ${color} 38%, transparent)`,
            boxShadow: `0 0 6px color-mix(in srgb, ${color} 19%, transparent)`,
          } : undefined}
        >
          {isEnabled && <span className="w-1.5 h-1.5 rounded-full bg-white/80 flex-shrink-0" />}
          {label}
        </button>
        {params.hasWet && (
          <DJEQKnob
            label={params.wet.label}
            valueText={wetText}
            value={params.wet.value}
            onChange={handleKnobWet}
            color={isEnabled ? enabledColor : 'var(--dj-border-hover)'}
            size={32}
            className="dj-fx-knob"
          />
        )}
      </div>
    );
  }

  return (
    <div className={`flex flex-col items-center gap-1 px-1.5 py-1 rounded-md transition-all duration-150 ${
      isEnabled ? 'bg-[var(--dj-surface-3)] border border-current/20' : 'bg-[var(--dj-surface-1)] border border-transparent'
    }`}
      style={isEnabled ? { borderColor: `color-mix(in srgb, ${color} 19%, transparent)` } : undefined}
    >
      {/* Enable/Disable button */}
      <button
        onClick={handleToggle}
        aria-pressed={isEnabled}
        className={`
          w-full px-2 py-1.5 rounded text-[12px] font-bold uppercase tracking-wider
          transition-all duration-100 border flex items-center justify-center gap-1 min-h-[36px]
          ${isEnabled
            ? 'text-white border-transparent'
            : 'bg-[var(--dj-surface-3)] text-[var(--dj-text-secondary)] border-[var(--dj-border-light)] hover:text-[var(--dj-text-secondary)] hover:border-[var(--dj-border-hover)]'}
        `}
        style={isEnabled ? {
          backgroundColor: color,
          boxShadow: `0 0 8px color-mix(in srgb, ${color} 25%, transparent)`,
        } : undefined}
      >
        {isEnabled && <span className="w-1.5 h-1.5 rounded-full bg-white/80 flex-shrink-0" />}
        {label}
      </button>

      {/* Knobs row */}
      <div className="flex gap-1.5">
        <DJEQKnob
          label={params.param1.label}
          valueText={valueText1}
          value={params.param1.value}
          onChange={handleKnobParam1}
          color={isEnabled ? enabledColor : 'var(--dj-text-muted)'}
          size={knobSize}
          className="dj-fx-knob"
        />
        <DJEQKnob
          label={params.param2.label}
          valueText={valueText2}
          value={params.param2.value}
          onChange={handleKnobParam2}
          color={isEnabled ? enabledColor : 'var(--dj-text-muted)'}
          size={knobSize}
          className="dj-fx-knob"
        />
        {params.hasWet && (
          <DJEQKnob
            label={params.wet.label}
            valueText={wetText}
            value={params.wet.value}
            onChange={handleKnobWet}
            color={isEnabled ? enabledColor : 'var(--dj-text-muted)'}
            size={knobSize}
            className="dj-fx-knob"
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
  const isCompact = layoutMode === 'browse';
  const isExpanded = layoutMode === 'fx';

  // Count active effects per deck
  const activeA = useStore(state =>
    Object.values(state.djDeckA.fx).filter(fx => fx.enabled).length);
  const activeB = useStore(state =>
    Object.values(state.djDeckB.fx).filter(fx => fx.enabled).length);

  return (
    <div className={`bg-[var(--dj-surface-2)] border-b border-[var(--dj-border)] ${className}`}>
      {/* Header */}
      <div
        role="button" tabIndex={0} aria-label="Toggle effects" aria-expanded={!collapsed}
        onKeyDown={event => { if (event.key === 'Enter' || event.key === ' ') { event.preventDefault(); setCollapsed(value => !value); } }}
        className="flex items-center justify-between px-4 h-6 cursor-pointer select-none hover:bg-white/5 transition-colors"
        onClick={() => setCollapsed(!collapsed)}
      >
        <div className="flex items-center gap-3">
          <span className="text-[12px] font-bold text-[var(--dj-text-secondary)] uppercase tracking-widest">FX</span>
          {activeA > 0 && (
            <span className="text-[12px] font-bold text-[var(--dj-deck-a-bright)] bg-[color-mix(in_srgb,var(--dj-deck-a)_15%,transparent)] px-1.5 py-0.5 rounded">
              A: {activeA}
            </span>
          )}
          {activeB > 0 && (
            <span className="text-[12px] font-bold text-[var(--dj-deck-b-bright)] bg-[color-mix(in_srgb,var(--dj-deck-b)_15%,transparent)] px-1.5 py-0.5 rounded">
              B: {activeB}
            </span>
          )}
        </div>
        {collapsed ? (
          <ChevronDown size={12} className="text-[var(--dj-text-secondary)]" />
        ) : (
          <ChevronUp size={12} className="text-[var(--dj-text-secondary)]" />
        )}
      </div>

      {/* FX Strip — Deck A FX | X-Y FX Pad (centre, fills the dead space) | Deck B FX */}
      {!collapsed && (
        <div className={`flex items-stretch px-2 pb-2 gap-2 ${isExpanded ? 'pt-1' : ''}`}>
          {/* Deck A FX */}
          <div className={`flex-1 flex justify-end ${isCompact ? 'items-center' : ''} gap-1 bg-[var(--dj-surface-1)] rounded-md p-1.5 border border-[var(--dj-surface-3)]`}>
            <div className="text-[12px] font-bold text-[var(--dj-deck-a-bright)] writing-vertical flex items-center justify-center w-3 mr-0.5"
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
          <div className={`flex-shrink-0 flex items-center justify-center bg-[var(--dj-bg)] rounded-md border border-[var(--dj-surface-3)] px-2 py-1 ${isExpanded ? 'gap-2' : ''}`}>
            {isExpanded && <DJBeatFXPanel />}
            <DJFXPad size={isExpanded ? 150 : 96} />
          </div>

          {/* Deck B FX */}
          <div className={`flex-1 flex ${isCompact ? 'items-center' : ''} gap-1 bg-[var(--dj-surface-1)] rounded-md p-1.5 border border-[var(--dj-surface-3)]`}>
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
            <div className="text-[12px] font-bold text-[var(--dj-deck-b-bright)] writing-vertical flex items-center justify-center w-3 ml-0.5"
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
