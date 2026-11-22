/** Owns DJ library drawer visibility, focus, and viewport sizing. */

import React, { forwardRef, useCallback, useEffect, useImperativeHandle, useRef, useState } from 'react';
import { Library, Maximize2, Minimize2, X } from 'lucide-react';
import { DJLibraryBrowserV2 } from './DJLibraryBrowserV2';
import { DJErrorBoundary } from './DJErrorBoundary';
import { registerDJMidiLibraryOpener } from '../../../lib/djMidiLibrary';

export interface DJLibraryDrawerHandle {
  open: () => void;
  close: () => boolean;
  toggle: () => void;
}

interface DJLibraryDrawerProps {
  /** Mirrors open state to other controls (e.g. the top-bar LIBRARY button). */
  onOpenChange?: (open: boolean) => void;
}

/** Local UI state never reaches the deck tree or audio store. */
export const DJLibraryDrawer = forwardRef<DJLibraryDrawerHandle, DJLibraryDrawerProps>(function DJLibraryDrawer({ onOpenChange }, ref) {
  const [open, setOpen] = useState(false);
  const [expanded, setExpanded] = useState(false);
  const [visited, setVisited] = useState(false);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const drawerRef = useRef<HTMLElement>(null);
  const returnFocusRef = useRef<HTMLElement | null>(null);

  useEffect(() => { onOpenChange?.(open); }, [open, onOpenChange]);

  const show = useCallback(() => {
    if (!open) returnFocusRef.current = document.activeElement as HTMLElement | null;
    setVisited(true);
    setOpen(true);
    // Also handles '/' when the drawer is already open.
    drawerRef.current?.querySelector<HTMLInputElement>('input[type="text"]')?.focus({ preventScroll: true });
  }, [open]);

  const close = useCallback(() => {
    if (!open) return false;
    setOpen(false);
    const target = returnFocusRef.current;
    if (target?.isConnected && target !== document.body && !drawerRef.current?.contains(target)) {
      target.focus({ preventScroll: true });
    } else {
      triggerRef.current?.focus({ preventScroll: true });
    }
    return true;
  }, [open]);

  const toggle = useCallback(() => { if (open) close(); else show(); }, [open, close, show]);
  const showRef = useRef(show);
  const toggleRef = useRef(toggle);
  showRef.current = show;
  toggleRef.current = toggle;
  useEffect(() => registerDJMidiLibraryOpener(() => showRef.current(), () => toggleRef.current()), []);

  useImperativeHandle(ref, () => ({ open: show, close, toggle }), [show, close, toggle]);

  return (
    <>
      <div className="dj-library-affordance">
        <button ref={triggerRef} type="button" className="dj-btn dj-focus-ring"
          aria-controls="dj-library-drawer" aria-expanded={open} onClick={toggle}>
          <Library size={16} aria-hidden="true" /> <span>Library</span> <kbd className="dj-kbd">/</kbd>
        </button>
      </div>
      <section ref={drawerRef} id="dj-library-drawer" role="region" aria-label="DJ library" hidden={!open} aria-hidden={!open}
        data-size={expanded ? 'expanded' : 'compact'}
        className={`dj-library-drawer ${expanded ? '' : 'max-h-[38%]'}`}>
        <header className="flex items-center justify-between px-3 border-b border-surface-3 shrink-0">
          <h2 className="text-xs font-bold text-text-main">Library</h2>
          <div className="flex items-center gap-1">
            <button type="button" aria-label={expanded ? 'Compact library height' : 'Expand library height'} aria-pressed={expanded}
              title={expanded ? 'Compact height (keep decks visible)' : 'Expand library height'}
              onClick={() => setExpanded(prev => !prev)}
              className="dj-focus-ring min-h-8 min-w-8 flex items-center justify-center text-text-secondary hover:text-text-main">
              {expanded ? <Minimize2 size={14} aria-hidden="true" /> : <Maximize2 size={14} aria-hidden="true" />}
            </button>
            <button type="button" aria-label="Close library" onClick={close}
              className="dj-focus-ring min-h-8 min-w-8 flex items-center justify-center text-text-secondary hover:text-text-main">
              <X size={16} aria-hidden="true" />
            </button>
          </div>
        </header>
        <div className="flex-1 min-h-0 overflow-hidden">
          {visited && <DJErrorBoundary componentName="DJLibraryBrowserV2">
            <DJLibraryBrowserV2 autoFocusSearch={open} />
          </DJErrorBoundary>}
        </div>
      </section>
    </>
  );
});
