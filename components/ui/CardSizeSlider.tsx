import React from 'react';
import { LayoutGrid } from 'lucide-react';

interface CardSizeSliderProps {
  value: number;
  onChange: (value: number) => void;
  min?: number;
  max?: number;
}

/**
 * A slider that lets the user control card size on grid pages.
 * Sliding RIGHT makes cards LARGER (fewer columns).
 * Sliding LEFT makes cards SMALLER (more columns).
 *
 * `value` is still the column count stored by the parent; the inversion
 * is handled internally so the visual direction feels natural.
 */
export const CardSizeSlider: React.FC<CardSizeSliderProps> = ({
  value,
  onChange,
  min = 2,
  max = 8,
}) => {
  // Invert so slider-right → fewer cols → larger cards.
  const sliderValue = max + min - value;

  return (
    <div className="flex items-center gap-2 text-text-subtle" title="Card size">
      <LayoutGrid size={15} className="shrink-0" />
      <input
        type="range"
        min={min}
        max={max}
        step={1}
        value={sliderValue}
        onChange={(e) => onChange(max + min - Number(e.target.value))}
        className="w-24 h-1 accent-brand cursor-pointer"
        aria-label="Card size"
      />
    </div>
  );
};
