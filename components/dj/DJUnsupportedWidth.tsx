import React from 'react';
import { Monitor, ArrowLeft } from 'lucide-react';
import { useNavigate } from 'react-router-dom';

export interface DJUnsupportedWidthProps {
  minWidth: number;
  variant?: 'v1' | 'v2';
}

/**
 * Deliberate unsupported-width state for DJ pages. Shown below the minimum
 * supported viewport instead of clipping the workstation.
 */
export const DJUnsupportedWidth: React.FC<DJUnsupportedWidthProps> = ({ minWidth, variant = 'v2' }) => {
  const navigate = useNavigate();
  const label = variant === 'v2' ? 'DJ Mode v2' : 'DJ Mode';

  return (
    <div
      className="h-full flex flex-col items-center justify-center px-6 text-center bg-surface-0"
      role="status"
      aria-live="polite"
    >
      <div className="max-w-sm flex flex-col items-center gap-4">
        <div className="p-4 rounded-full bg-surface-2 text-brand">
          <Monitor size={36} aria-hidden="true" />
        </div>
        <h1 className="text-xl font-bold text-text-main">{label} needs a wider screen</h1>
        <p className="text-sm text-text-secondary">
          {label} is designed for a desktop canvas of at least <strong>{minWidth}px</strong> wide.
          Try resizing this window, rotating to landscape, or opening ViiB on a larger display.
        </p>
        <button
          type="button"
          onClick={() => navigate('/')}
          className="mt-2 inline-flex items-center gap-2 px-4 py-2 rounded-lg bg-brand text-black font-semibold hover:opacity-90 transition-opacity"
        >
          <ArrowLeft size={16} aria-hidden="true" />
          Back to Home
        </button>
      </div>
    </div>
  );
};
