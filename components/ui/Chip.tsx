import React from 'react';
import { cn } from './cn';
import { ViibAccent, accentToRingClass } from './tokens';

export interface ChipProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  selected?: boolean;
  accent?: ViibAccent;
}

export const Chip: React.FC<ChipProps> = ({
  selected = false,
  accent = 'brand',
  className,
  children,
  type,
  ...rest
}) => {
  return (
    <button
      type={type ?? 'button'}
      className={cn(
        'inline-flex items-center justify-center rounded-full px-3 py-1.5 text-[12px] ' +
          'transition-all duration-150 ease-out motion-reduce:transition-none ' +
          'bg-surface-2 text-text-secondary ring-1 ring-surface-3/80 ' +
          'hover:text-text-main hover:ring-white/10',
        selected && cn('text-text-main', 'ring-2', accentToRingClass(accent)),
        className
      )}
      {...rest}
    >
      {children}
    </button>
  );
};
