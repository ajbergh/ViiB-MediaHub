/** Defines the shared button variants, loading state, and interaction styling. */

import React from 'react';
import { cn } from './cn';
import { ViibAccent, accentToBgClass } from './tokens';

export type ButtonVariant = 'primary' | 'secondary' | 'ghost';

export type ButtonSize = 'sm' | 'md' | 'lg';

export interface ButtonProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant;
  accent?: ViibAccent;
  size?: ButtonSize;
  leftIcon?: React.ReactNode;
  rightIcon?: React.ReactNode;
}

export const Button: React.FC<ButtonProps> = ({
  variant = 'secondary',
  accent = 'brand',
  size = 'md',
  leftIcon,
  rightIcon,
  className,
  children,
  type,
  ...rest
}) => {
  const sizeClass =
    size === 'sm'
      ? 'px-3 py-1.5 text-xs'
      : size === 'lg'
      ? 'px-6 py-2.5 text-sm'
      : 'px-4 py-2 text-[13px]';

  const base =
    'inline-flex items-center justify-center gap-2 select-none ' +
    `rounded-full font-semibold ${sizeClass} ` +
    'transition-all duration-150 ease-out ' +
    'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand focus-visible:ring-offset-2 focus-visible:ring-offset-surface-0 ' +
    'disabled:opacity-50 disabled:pointer-events-none ' +
    'motion-reduce:transition-none';

  const primary = cn(
    accentToBgClass(accent),
    'text-surface-0 font-bold',
    'hover:brightness-105 hover:scale-[1.02] active:scale-[0.98]'
  );

  const secondary =
    'bg-surface-3 text-text-main ' +
    'hover:bg-surface-highlight';

  const ghost =
    'bg-transparent text-text-secondary ' +
    'hover:bg-surface-hover hover:text-text-main';

  const variantClass =
    variant === 'primary' ? primary : variant === 'secondary' ? secondary : ghost;

  return (
    <button
      type={type ?? 'button'}
      className={cn(base, variantClass, className)}
      {...rest}
    >
      {leftIcon}
      {children}
      {rightIcon}
    </button>
  );
};
