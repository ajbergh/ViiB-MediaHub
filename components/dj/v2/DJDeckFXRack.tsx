/**
 * ViiB MediaHub - DJ Deck FX Rack (v2)
 *
 * Compact deck-local effects row: Filter, Delay, Reverb and Flanger, each a
 * label toggle over its parameter knobs (Plan §7, §10A.12). Reuses the
 * existing FXUnit store/engine wiring from DJFXSection. The X-Y pad and Beat
 * FX live in the mixer's FX tab.
 *
 * @module components/dj/v2/DJDeckFXRack
 */

import React from 'react';
import type { DeckId, EffectType } from '../../../slices/djMixerSlice';
import { FXUnit, FX_CONFIGS } from './DJFXSection';

export const DJDeckFXRack = React.memo(function DJDeckFXRack({ deck }: { deck: DeckId }) {
  return (
    <section className='dj-fx-rack' aria-label={`Deck ${deck} effects`} data-dj-fx-rack={deck}>
      <span className='dj-fx-rack-label' aria-hidden='true'>FX</span>
      {(Object.keys(FX_CONFIGS) as EffectType[]).map(type => (
        <FXUnit key={type} deck={deck} type={type} label={FX_CONFIGS[type].label}
          color={FX_CONFIGS[type].color} enabledColor={FX_CONFIGS[type].enabledColor} rack />
      ))}
    </section>
  );
});
