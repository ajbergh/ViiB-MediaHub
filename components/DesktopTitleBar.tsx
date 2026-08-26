/** Native-window controls for the frameless desktop shell. */

import React from 'react';
import { Maximize2, Minus, X } from 'lucide-react';
import {
  hideNativeWindow,
  isNativeWindowRuntimeAvailable,
  minimiseNativeWindow,
  toggleNativeWindowMaximise,
} from '../services/skinnyWindowService';

const noDragStyle = { '--wails-draggable': 'no-drag' } as React.CSSProperties;
const dragStyle = { '--wails-draggable': 'drag' } as React.CSSProperties;

export const DesktopTitleBar: React.FC = () => {
  if (!isNativeWindowRuntimeAvailable()) return null;

  return (
    <header
      className="flex h-8 flex-shrink-0 items-center justify-between border-b border-surface-3 bg-surface-0 pl-3 text-xs text-text-main"
      style={dragStyle}
    >
      <span className="font-medium">ViiB MediaHub</span>
      <div className="flex h-full" style={noDragStyle}>
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
          onClick={() => void toggleNativeWindowMaximise()}
          className="flex h-8 w-11 items-center justify-center text-text-secondary transition-colors hover:bg-surface-2 hover:text-text-main"
          aria-label="Maximise or restore window"
          title="Maximise or restore"
        >
          <Maximize2 size={14} aria-hidden="true" />
        </button>
        <button
          type="button"
          onClick={() => void hideNativeWindow()}
          className="flex h-8 w-11 items-center justify-center text-text-secondary transition-colors hover:bg-accent-crimson hover:text-surface-0"
          aria-label="Hide window to system tray"
          title="Hide to system tray"
        >
          <X size={16} aria-hidden="true" />
        </button>
      </div>
    </header>
  );
};
