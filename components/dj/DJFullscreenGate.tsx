import React, { useState, useCallback } from 'react';
import { Maximize2, ArrowLeft, Monitor } from 'lucide-react';
import { useNavigate } from 'react-router';

interface DJFullscreenGateProps {
  children: React.ReactNode;
}

/**
 * Wraps DJ Mode v2 content. When the viewport is below 1920×1080 AND
 * the document is not in fullscreen, renders an informational overlay
 * offering the user the option to enter fullscreen or continue anyway.
 *
 * The gate is skipped entirely once the user dismisses it for the session
 * or after entering fullscreen.
 */
export const DJFullscreenGate: React.FC<DJFullscreenGateProps> = ({ children }) => {
  const navigate = useNavigate();
  const [dismissed, setDismissed] = useState(false);

  // Re-check on every render so the overlay auto-hides when fullscreen is entered
  const isFullscreen = typeof document !== 'undefined' ? !!document.fullscreenElement : false;
  const viewportW = typeof window !== 'undefined' ? window.innerWidth : 0;
  const viewportH = typeof window !== 'undefined' ? window.innerHeight : 0;
  const is1080p = viewportW >= 1920 && viewportH >= 1080;

  const showGate = !dismissed && !is1080p && !isFullscreen;

  const handleEnterFullscreen = useCallback(async () => {
    try {
      await document.documentElement.requestFullscreen();
      setDismissed(true);
    } catch {
      // Fullscreen API not available or denied — let the user continue anyway
      setDismissed(true);
    }
  }, []);

  if (!showGate) {
    return <>{children}</>;
  }

  return (
    <div
      className="h-full flex flex-col items-center justify-center px-6 text-center bg-surface-0"
      role="dialog"
      aria-modal="true"
      aria-labelledby="djgate-title"
      aria-describedby="djgate-desc"
    >
      <div className="max-w-md flex flex-col items-center gap-5">
        {/* Icon */}
        <div className="p-5 rounded-full bg-surface-2 text-brand">
          <Monitor size={40} aria-hidden="true" />
        </div>

        {/* Heading */}
        <h1 id="djgate-title" className="text-2xl font-bold text-text-main">
          DJ Mode v2 works best at 1080p
        </h1>

        {/* Description */}
        <p id="djgate-desc" className="text-sm text-text-secondary leading-relaxed max-w-sm">
          This panel is designed for a{' '}
          <strong className="text-text-main">1920 × 1080</strong> (Full HD) canvas.
          Your current viewport is{' '}
          <strong className="text-text-main">{viewportW} × {viewportH}</strong>.
          Enter fullscreen to unlock the full experience, or continue in the current
          window size.
        </p>

        {/* Resolution chips */}
        <div className="flex gap-2 text-xs text-text-subtle">
          <span className="px-2 py-1 bg-surface-2 rounded">Current: {viewportW}×{viewportH}</span>
          <span className="px-2 py-1 bg-surface-2 rounded">Target: 1920×1080</span>
        </div>

        {/* Primary CTA */}
        <button
          type="button"
          onClick={handleEnterFullscreen}
          className="w-full max-w-xs inline-flex items-center justify-center gap-2
            px-6 py-3 rounded-xl bg-brand text-black font-bold text-sm
            hover:opacity-90 active:scale-95 transition-all"
        >
          <Maximize2 size={16} aria-hidden="true" />
          Enter Fullscreen (F11)
        </button>

        {/* Secondary actions */}
        <div className="flex flex-col sm:flex-row items-center gap-3 text-sm">
          <button
            type="button"
            onClick={() => setDismissed(true)}
            className="text-text-subtle hover:text-text-main underline underline-offset-2 transition-colors"
          >
            Continue anyway
          </button>
          <span className="text-text-subtle hidden sm:inline">·</span>
          <button
            type="button"
            onClick={() => navigate('/')}
            className="inline-flex items-center gap-1.5 text-text-subtle hover:text-text-main transition-colors"
          >
            <ArrowLeft size={14} aria-hidden="true" />
            Back to Home
          </button>
        </div>
      </div>
    </div>
  );
};
