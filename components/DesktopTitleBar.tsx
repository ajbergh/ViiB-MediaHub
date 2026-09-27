/** Native-window controls for the frameless desktop shell. */

import React, { useEffect, useState } from 'react';
import { Maximize2, Minimize2, Minus, X } from 'lucide-react';
import {
  closeNativeWindow,
  isNativeWindowRuntimeAvailable,
  minimiseNativeWindow,
  toggleNativeWindowMaximise,
} from '../services/skinnyWindowService';
import { useStore } from '../store';
import { isMacOSWails } from '../lib/webglSafety';

const noDragStyle = { '--wails-draggable': 'no-drag' } as React.CSSProperties;
const dragStyle = { '--wails-draggable': 'drag' } as React.CSSProperties;

export const DesktopTitleBar: React.FC = () => {
  const closeAction = useStore((state) => state.windowCloseAction);
  const setSkinnyMode = useStore((state) => state.setSkinnyMode);
  const [isMaximised, setIsMaximised] = useState(false);

  useEffect(() => {
    let active = true;
    const updateMaximisedState = async () => {
      try {
        const runtime = await import('../backend/cmd/wails/frontend/wailsjs/runtime/runtime');
        const maximised = await runtime.WindowIsMaximised();
        if (active) setIsMaximised(maximised);
      } catch {
        // The control remains usable if the platform cannot report this state.
      }
    };
    void updateMaximisedState();
    window.addEventListener('resize', updateMaximisedState);
    return () => {
      active = false;
      window.removeEventListener('resize', updateMaximisedState);
    };
  }, []);

  // macOS uses Wails' native title bar and traffic-light controls instead of
  // duplicating them with the cross-platform custom title bar.
  if (!isNativeWindowRuntimeAvailable() || isMacOSWails()) return null;

  const closeLabel = closeAction === 'quit' ? 'Close app' : 'Hide window';

  return (
    <header
      className="flex h-8 flex-shrink-0 items-center justify-between border-b border-surface-3 bg-surface-0 pl-3 text-xs text-text-main"
      style={dragStyle}
      onDoubleClick={() => void toggleNativeWindowMaximise()}
    >
      <span className="font-medium">ViiB MediaHub</span>
      <div className="flex h-full" style={noDragStyle} onDoubleClick={(event) => event.stopPropagation()}>
        <button
          type="button"
          onClick={() => setSkinnyMode(true)}
          className="flex h-8 w-11 items-center justify-center text-text-secondary transition-colors hover:bg-surface-2 hover:text-text-main"
          aria-label="Enter skinny player mode"
          title="Enter skinny player mode"
        >
          <Minimize2 size={15} aria-hidden="true" />
        </button>
        <button
          type="button"
          onClick={() => void minimiseNativeWindow()}
          className="flex h-8 w-11 items-center justify-center text-text-secondary transition-colors hover:bg-surface-2 hover:text-text-main"
          aria-label="Minimise window"
          title="Minimise"
        >
          <Minus size={15} aria-hidden="true" />
        </button>
        <button
          type="button"
          onClick={() => {
            void toggleNativeWindowMaximise();
            setIsMaximised((current) => !current);
          }}
          className="flex h-8 w-11 items-center justify-center text-text-secondary transition-colors hover:bg-surface-2 hover:text-text-main"
          aria-label={isMaximised ? 'Restore window' : 'Maximise window'}
          title={isMaximised ? 'Restore' : 'Maximise'}
        >
          {isMaximised ? <Minimize2 size={14} aria-hidden="true" /> : <Maximize2 size={14} aria-hidden="true" />}
        </button>
        <button
          type="button"
          onClick={() => void closeNativeWindow(closeAction)}
          className="flex h-8 w-11 items-center justify-center text-text-secondary transition-colors hover:bg-accent-crimson hover:text-surface-0"
          aria-label={closeLabel}
          title={closeLabel}
        >
          <X size={16} aria-hidden="true" />
        </button>
      </div>
    </header>
  );
};
