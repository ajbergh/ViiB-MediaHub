import React from 'react';
import { LayoutGrid, List } from 'lucide-react';

export const MAX_CARD_COLUMNS = 8;

interface CardSizeSliderProps {
  value: number;
  onChange: (value: number) => void;
  min?: number;
  max?: number;
  listAtMinimum?: boolean;
}

/**
 * A slider that lets the user control card size on grid pages.
 * Sliding RIGHT makes cards LARGER (fewer columns).
 * Sliding LEFT makes cards SMALLER (more columns).
 * With listAtMinimum, the leftmost position selects list view.
 *
 * `value` is still the column count stored by the parent; the inversion
 * is handled internally so the visual direction feels natural.
 */
export const CardSizeSlider: React.FC<CardSizeSliderProps> = ({
  value,
  onChange,
  min = 2,
  max = MAX_CARD_COLUMNS,
  listAtMinimum = false,
}) => {
  // Invert so slider-right → fewer cols → larger cards.
  const sliderValue = max + min - value;

  const listView = listAtMinimum && value === max;
  const Icon = listView ? List : LayoutGrid;

  return (
    <div className="flex items-center gap-2 text-text-subtle" title={listAtMinimum ? 'View and card size (leftmost: list view)' : 'Card size'}>
      <Icon size={15} className="shrink-0" />
      <input
        type="range"
        min={min}
        max={max}
        step={1}
        value={sliderValue}
        onChange={(e) => onChange(max + min - Number(e.target.value))}
        className="w-24 h-1 accent-brand cursor-pointer"
        aria-label="Card size"
        aria-valuetext={listView ? 'List view' : `${value} columns`}
      />
    </div>
  );
};
