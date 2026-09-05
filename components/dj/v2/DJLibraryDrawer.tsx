import React, { forwardRef, useCallback, useImperativeHandle, useRef, useState } from 'react';
import { Library, X } from 'lucide-react';
import { DJLibraryBrowserV2 } from './DJLibraryBrowserV2';
import { DJErrorBoundary } from './DJErrorBoundary';

export interface DJLibraryDrawerHandle {
  open: () => void;
  close: () => boolean;
}

/** Local UI state never reaches the deck tree or audio store. */
export const DJLibraryDrawer = forwardRef<DJLibraryDrawerHandle>(function DJLibraryDrawer(_, ref) {
  const [open, setOpen] = useState(false);
  const [visited, setVisited] = useState(false);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const drawerRef = useRef<HTMLElement>(null);
  const returnFocusRef = useRef<HTMLElement | null>(null);

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

  useImperativeHandle(ref, () => ({ open: show, close }), [show, close]);

  return (
    <>
      <div className="dj-library-affordance">
        <button ref={triggerRef} type="button" className="dj-focus-ring inline-flex items-center gap-2 px-5 min-h-8 text-xs text-text-main"
          aria-controls="dj-library-drawer" aria-expanded={open} onClick={() => open ? close() : show()}>
          <Library size={14} aria-hidden="true" /> Library <span className="text-text-subtle">/</span>
        </button>
      </div>
      <section ref={drawerRef} id="dj-library-drawer" role="region" aria-label="DJ library" hidden={!open}
        className="dj-library-drawer">
        <header className="flex items-center justify-between px-3 border-b border-surface-3 shrink-0">
          <h2 className="text-xs font-bold text-text-main">Library</h2>
          <button type="button" aria-label="Close library" onClick={close}
            className="dj-focus-ring min-h-8 min-w-8 flex items-center justify-center text-text-secondary hover:text-text-main">
            <X size={16} aria-hidden="true" />
          </button>
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
