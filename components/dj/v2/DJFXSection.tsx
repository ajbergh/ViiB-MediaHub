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
import { useDJAudioEngine } from '../../../hooks/useDJAudioEngine';
import { DJEQKnob } from './DJEQKnob';
import type { DeckId, EffectType } from '../../../slices/djMixerSlice';
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
}

const FX_CONFIGS = {
  filter: { label: 'FILTER', color: '#3b82f6', enabledColor: '#60a5fa' },
  delay: { label: 'DELAY', color: '#22c55e', enabledColor: '#4ade80' },
  reverb: { label: 'REVERB', color: '#a855f7', enabledColor: '#c084fc' },
  flanger: { label: 'FLANGER', color: '#f97316', enabledColor: '#fb923c' },
} as const;

const FXUnit = memo<FXUnitProps>(({ deck, type, label, color, enabledColor }) => {
  const fx = useStore(state => {
    const deckState = deck === 'A' ? state.djDeckA : state.djDeckB;
    return deckState.fx[type];
  });

  const { setFilterFX: storeSetFilterFX, setDelayFX: storeSetDelayFX,
          setReverbFX: storeSetReverbFX, setFlangerFX: storeSetFlangerFX,
          toggleFX } = useStore();
  const { setFilterFX, setDelayFX, setFlangerFX, setReverbFX } = useDJAudioEngine();

  const isEnabled = fx.enabled;

  // Toggle enable/disable
  const handleToggle = useCallback(() => {
    const newEnabled = !fx.enabled;
    toggleFX(deck, type);

    switch (type) {
      case 'filter': {
        const f = fx as any;
        setFilterFX(deck, newEnabled, f.type || 'lowpass', f.frequency || 1000, f.resonance || 5);
        break;
      }
      case 'delay': {
        const d = fx as any;
        setDelayFX(deck, newEnabled, d.time || 0.25, d.feedback || 0.3, d.mix || 0.5);
        break;
      }
      case 'reverb': {
        const r = fx as any;
        setReverbFX(deck, newEnabled, r.roomSize || 0.5, r.damping || 0.5, r.mix || 0.3);
        break;
      }
      case 'flanger': {
        const fl = fx as any;
        setFlangerFX(deck, newEnabled, fl.rate || 0.5, fl.depth || 0.5, fl.feedback || 0.3);
        break;
      }
    }
  }, [deck, type, fx, toggleFX, setFilterFX, setDelayFX, setReverbFX, setFlangerFX]);

  // Parameter change handlers - specific to FX type
  const handleParam1Change = useCallback((value: number) => {
    switch (type) {
      case 'filter': {
        const f = fx as any;
        storeSetFilterFX(deck, { frequency: value });
        if (f.enabled) setFilterFX(deck, true, f.type, value, f.resonance);
        break;
      }
      case 'delay': {
        const d = fx as any;
        storeSetDelayFX(deck, { time: value });
        if (d.enabled) setDelayFX(deck, true, value, d.feedback, d.mix);
        break;
      }
      case 'reverb': {
        const r = fx as any;
        storeSetReverbFX(deck, { roomSize: value });
        if (r.enabled) setReverbFX(deck, true, value, r.damping, r.mix);
        break;
      }
      case 'flanger': {
        const fl = fx as any;
        storeSetFlangerFX(deck, { rate: value });
        if (fl.enabled) setFlangerFX(deck, true, value, fl.depth, fl.feedback);
        break;
      }
    }
  }, [deck, type, fx, storeSetFilterFX, storeSetDelayFX, storeSetReverbFX, storeSetFlangerFX,
      setFilterFX, setDelayFX, setReverbFX, setFlangerFX]);

  const handleParam2Change = useCallback((value: number) => {
    switch (type) {
      case 'filter': {
        const f = fx as any;
        storeSetFilterFX(deck, { resonance: value });
        if (f.enabled) setFilterFX(deck, true, f.type, f.frequency, value);
        break;
      }
      case 'delay': {
        const d = fx as any;
        storeSetDelayFX(deck, { feedback: value });
        if (d.enabled) setDelayFX(deck, true, d.time, value, d.mix);
        break;
      }
      case 'reverb': {
        const r = fx as any;
        storeSetReverbFX(deck, { damping: value });
        if (r.enabled) setReverbFX(deck, true, r.roomSize, value, r.mix);
        break;
      }
      case 'flanger': {
        const fl = fx as any;
        storeSetFlangerFX(deck, { depth: value });
        if (fl.enabled) setFlangerFX(deck, true, fl.rate, value, fl.feedback);
        break;
      }
    }
  }, [deck, type, fx, storeSetFilterFX, storeSetDelayFX, storeSetReverbFX, storeSetFlangerFX,
      setFilterFX, setDelayFX, setReverbFX, setFlangerFX]);

  const handleDryWetChange = useCallback((value: number) => {
    switch (type) {
      case 'filter': {
        // Filter doesn't have mix - use resonance as secondary
        break;
      }
      case 'delay': {
        const d = fx as any;
        storeSetDelayFX(deck, { mix: value });
        if (d.enabled) setDelayFX(deck, true, d.time, d.feedback, value);
        break;
      }
      case 'reverb': {
        const r = fx as any;
        storeSetReverbFX(deck, { mix: value });
        if (r.enabled) setReverbFX(deck, true, r.roomSize, r.damping, value);
        break;
      }
      case 'flanger': {
        const fl = fx as any;
        storeSetFlangerFX(deck, { feedback: value });
        if (fl.enabled) setFlangerFX(deck, true, fl.rate, fl.depth, value);
        break;
      }
    }
  }, [deck, type, fx, storeSetDelayFX, storeSetReverbFX, storeSetFlangerFX,
      setDelayFX, setReverbFX, setFlangerFX]);

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

  return (
    <div className={`flex flex-col items-center gap-1 px-1.5 py-1 rounded-md transition-all duration-150 ${
      isEnabled ? 'bg-[#1a1a1a]' : 'bg-[#141414]'
    }`}>
      {/* Enable/Disable button */}
      <button
        onClick={handleToggle}
        className={`
          w-full px-2 py-1 rounded text-[9px] font-bold uppercase tracking-wider
          transition-all duration-100 border
          ${isEnabled
            ? 'text-white border-transparent'
            : 'bg-[#222] text-neutral-600 border-[#333] hover:text-neutral-400 hover:border-[#444]'}
        `}
        style={isEnabled ? {
          backgroundColor: color,
          boxShadow: `0 0 8px ${color}40`,
        } : undefined}
      >
        {label}
      </button>

      {/* Knobs row */}
      <div className="flex gap-0.5">
        <DJEQKnob
          label={params.param1.label}
          value={params.param1.value}
          onChange={handleKnobParam1}
          color={isEnabled ? enabledColor : '#555'}
          size={30}
        />
        <DJEQKnob
          label={params.param2.label}
          value={params.param2.value}
          onChange={handleKnobParam2}
          color={isEnabled ? enabledColor : '#555'}
          size={30}
        />
        {params.hasWet && (
          <DJEQKnob
            label={params.wet.label}
            value={params.wet.value}
            onChange={handleKnobWet}
            color={isEnabled ? enabledColor : '#555'}
            size={30}
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

  // Count active effects per deck
  const deckAFX = useStore(state => state.djDeckA.fx);
  const deckBFX = useStore(state => state.djDeckB.fx);

  const countActive = (fx: typeof deckAFX) =>
    [fx.filter.enabled, fx.delay.enabled, fx.reverb.enabled, fx.flanger.enabled].filter(Boolean).length;

  const activeA = countActive(deckAFX);
  const activeB = countActive(deckBFX);

  return (
    <div className={`bg-[#161616] border-b border-[#2a2a2a] ${className}`}>
      {/* Header */}
      <div
        className="flex items-center justify-between px-4 h-6 cursor-pointer select-none hover:bg-white/5 transition-colors"
        onClick={() => setCollapsed(!collapsed)}
      >
        <div className="flex items-center gap-3">
          <span className="text-[9px] font-bold text-[#555] uppercase tracking-widest">FX</span>
          {activeA > 0 && (
            <span className="text-[8px] font-bold text-blue-400 bg-blue-500/15 px-1.5 py-0.5 rounded">
              A: {activeA}
            </span>
          )}
          {activeB > 0 && (
            <span className="text-[8px] font-bold text-purple-400 bg-purple-500/15 px-1.5 py-0.5 rounded">
              B: {activeB}
            </span>
          )}
        </div>
        {collapsed ? (
          <ChevronDown size={12} className="text-neutral-600" />
        ) : (
          <ChevronUp size={12} className="text-neutral-600" />
        )}
      </div>

      {/* FX Strip */}
      {!collapsed && (
        <div className="flex items-stretch px-2 pb-2 gap-2">
          {/* Deck A FX */}
          <div className="flex-1 flex gap-1 bg-[#111] rounded-md p-1.5 border border-[#222]">
            <div className="text-[8px] font-bold text-blue-400 writing-vertical flex items-center justify-center w-3 mr-0.5"
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
              />
            ))}
          </div>

          {/* Deck B FX */}
          <div className="flex-1 flex gap-1 bg-[#111] rounded-md p-1.5 border border-[#222]">
            {(Object.keys(FX_CONFIGS) as EffectType[]).map(fxType => (
              <FXUnit
                key={`B-${fxType}`}
                deck="B"
                type={fxType}
                label={FX_CONFIGS[fxType].label}
                color={FX_CONFIGS[fxType].color}
                enabledColor={FX_CONFIGS[fxType].enabledColor}
              />
            ))}
            <div className="text-[8px] font-bold text-purple-400 writing-vertical flex items-center justify-center w-3 ml-0.5"
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
