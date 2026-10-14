/** Renders a shared label/filter chip with variant styling. */

import React from 'react';
import { cn } from './cn';
import { ViibAccent, accentToRingClass } from './tokens';

export interface ChipProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  selected?: boolean;
  accent?: ViibAccent;
}

export const Chip: React.FC<ChipProps> = ({
  selected = false,
  className,
  children,
  type,
  ...rest
}) => {
  return (
    <button
      type={type ?? 'button'}
      className={cn(
        'inline-flex items-center justify-center rounded-full px-3.5 py-1.5 text-[13px] font-medium ' +
          'transition-colors duration-150 ease-out motion-reduce:transition-none',
        selected
          ? 'bg-text-main text-surface-0 font-semibold'
          : 'bg-surface-3 text-text-main hover:bg-surface-highlight',
        className
      )}
      {...rest}
    >
      {children}
    </button>
  );
};
