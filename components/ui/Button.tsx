import React from 'react';
import { cn } from './cn';
import { ViibAccent, accentToBgClass } from './tokens';

export type ButtonVariant = 'primary' | 'secondary' | 'ghost';

export interface ButtonProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant;
  accent?: ViibAccent;
  leftIcon?: React.ReactNode;
  rightIcon?: React.ReactNode;
}

export const Button: React.FC<ButtonProps> = ({
  variant = 'secondary',
  accent = 'brand',
  leftIcon,
  rightIcon,
  className,
  children,
  type,
  ...rest
}) => {
  const base =
    'inline-flex items-center justify-center gap-2 select-none ' +
    'rounded-lg px-3 py-2 text-[13px] font-medium ' +
    'transition-all duration-150 ease-out ' +
    'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-offset-2 focus-visible:ring-offset-surface-0 ' +
    'disabled:opacity-50 disabled:pointer-events-none ' +
    'motion-reduce:transition-none';

  const primary = cn(
    accentToBgClass(accent),
    'text-black/90',
    'hover:shadow-lg hover:shadow-black/20',
    'hover:ring-1 hover:ring-white/10'
  );

  const secondary =
    'bg-surface-2 text-text-main ring-1 ring-surface-3 ' +
    'hover:bg-surface-2 hover:ring-white/10';

  const ghost =
    'bg-transparent text-text-secondary ' +
    'hover:bg-surface-2/60 hover:text-text-main';

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
