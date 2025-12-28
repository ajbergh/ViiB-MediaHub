/**
 * Menu / MenuItem — Viib Design System v1 Primitive
 *
 * Provides role="menu" and role="menuitem" with DS-aligned styling (surfaces, rings, text).
 *
 * Current state:
 * - Focus-visible styling implemented.
 * - Shared keyboard navigation (ArrowUp/Down, Home/End), typeahead, and Escape-to-close hooks.
 *
 * @module Menu
 */
import React, { useEffect, useMemo, useRef } from 'react';
import { cn } from './cn';

export interface MenuProps extends React.HTMLAttributes<HTMLDivElement> {
  autoFocusFirstItem?: boolean;
  onRequestClose?: () => void;
}

export const Menu: React.FC<MenuProps> = ({
  className,
  autoFocusFirstItem = true,
  onRequestClose,
  onKeyDown,
  ...rest
}) => {
  const menuRef = useRef<HTMLDivElement | null>(null);
  const typeaheadRef = useRef<{ buffer: string; lastAt: number }>({
    buffer: '',
    lastAt: 0,
  });

  const getEnabledMenuItems = useMemo(() => {
    return () => {
      const root = menuRef.current;
      if (!root) return [];
      return Array.from(
        root.querySelectorAll<HTMLButtonElement>(
          'button[role="menuitem"]:not([disabled])'
        )
      );
    };
  }, []);

  useEffect(() => {
    if (!autoFocusFirstItem) return;
    const items = getEnabledMenuItems();
    items[0]?.focus();
  }, [autoFocusFirstItem, getEnabledMenuItems]);

  const handleKeyDown = (e: React.KeyboardEvent<HTMLDivElement>) => {
    onKeyDown?.(e);
    if (e.defaultPrevented) return;

    const items = getEnabledMenuItems();
    if (items.length === 0) return;

    const active = document.activeElement as HTMLElement | null;
    const currentIndex = Math.max(
      0,
      items.findIndex((el) => el === active)
    );

    const focusIndex = (nextIndex: number) => {
      const clamped = Math.max(0, Math.min(items.length - 1, nextIndex));
      items[clamped]?.focus();
    };

    switch (e.key) {
      case 'ArrowDown':
        e.preventDefault();
        focusIndex((currentIndex + 1) % items.length);
        return;
      case 'ArrowUp':
        e.preventDefault();
        focusIndex((currentIndex - 1 + items.length) % items.length);
        return;
      case 'Home':
        e.preventDefault();
        focusIndex(0);
        return;
      case 'End':
        e.preventDefault();
        focusIndex(items.length - 1);
        return;
      case 'Escape':
        e.preventDefault();
        onRequestClose?.();
        return;
      case 'Tab':
        onRequestClose?.();
        return;
      default:
        break;
    }

    if (e.key.length === 1 && !e.ctrlKey && !e.metaKey && !e.altKey) {
      const now = Date.now();
      const windowMs = 600;
      const nextBuffer =
        now - typeaheadRef.current.lastAt > windowMs
          ? e.key
          : typeaheadRef.current.buffer + e.key;

      typeaheadRef.current = { buffer: nextBuffer.toLowerCase(), lastAt: now };

      const matchIndex = items.findIndex((el) =>
        (el.textContent || '').trim().toLowerCase().startsWith(typeaheadRef.current.buffer)
      );

      if (matchIndex >= 0) {
        focusIndex(matchIndex);
      }
    }
  };

  return (
    <div
      role="menu"
      aria-orientation="vertical"
      ref={menuRef}
      onKeyDown={handleKeyDown}
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
