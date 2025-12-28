/**
 * Menu / MenuItem — Viib Design System v1 Primitive
 *
 * Provides role="menu" and role="menuitem" with DS-aligned styling (surfaces, rings, text).
 *
 * Current state:
 * - Focus-visible styling implemented.
 * - Arrow-key roving focus and typeahead are NOT YET implemented (Phase 5 scope).
 *
 * @module Menu
 */
import React from 'react';
import { cn } from './cn';

export interface MenuProps extends React.HTMLAttributes<HTMLDivElement> {}

export const Menu: React.FC<MenuProps> = ({ className, ...rest }) => {
  return (
    <div
      role="menu"
      className={cn(
        'bg-surface-2 ring-1 ring-surface-3 rounded-xl shadow-xl shadow-black/30 ' +
          'py-1 overflow-hidden',
        className
      )}
      {...rest}
    />
  );
};

export interface MenuItemProps
  extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  active?: boolean;
}

export const MenuItem: React.FC<MenuItemProps> = ({
  active = false,
  className,
  ...rest
}) => {
  return (
    <button
      role="menuitem"
      className={cn(
        'w-full text-left px-4 py-2 text-[13px] transition-colors duration-150 ease-out ' +
          'text-text-main hover:bg-surface-1/60 ' +
          'focus-visible:outline-none focus-visible:bg-surface-1/60',
        active && 'text-brand font-medium',
        className
      )}
      {...rest}
    />
  );
};
