import React, { useEffect, useState } from 'react';
import { FolderOpen, Music, Server, Sparkles } from 'lucide-react';
import FirstLaunchDialogLegacy from './FirstLaunchDialogLegacy';
import PlexFirstLaunchSetup from './PlexFirstLaunchSetup';
import { Button } from './ui/Button';

interface FirstLaunchDialogProps {
  isOpen: boolean;
  onComplete: () => void;
}

type FirstLaunchPhase = 'sources' | 'local-and-integrations' | 'plex';

/**
 * First-run starts with a compact, explicit source choice. Each option then
 * opens only the steps needed for that source: local files use the established
 * folder and optional-integration flow, while Plex follows discovery, library
 * selection, and initial catalog import before setup can finish.
 */
export const FirstLaunchDialog: React.FC<FirstLaunchDialogProps> = ({ isOpen, onComplete }) => {
  const [phase, setPhase] = useState<FirstLaunchPhase>('sources');

  useEffect(() => {
    if (isOpen) setPhase('sources');
  }, [isOpen]);

  if (!isOpen) return null;

  if (phase === 'local-and-integrations') {
    return (
      <FirstLaunchDialogLegacy
        isOpen={isOpen}
        initialStep={2}
        onComplete={onComplete}
        onBackToSourceChoice={() => setPhase('sources')}
      />
    );
  }

  return (
    <div className="fixed inset-0 z-[200] flex items-center justify-center bg-black/90 px-4 py-4 backdrop-blur-sm animate-in fade-in duration-300">
      <div className="max-h-full w-full max-w-3xl overflow-y-auto rounded-2xl border border-surface-3 bg-surface-2 p-6 shadow-2xl sm:p-8">
        {phase === 'sources' ? (
          <>
            <div className="text-center">
              <div className="mb-4 flex justify-center">
                <div className="rounded-full border border-brand/20 bg-brand/10 p-4 shadow-lg shadow-brand/10">
                  <Music size={34} className="text-brand" />
                </div>
              </div>
              <p className="text-xs font-semibold uppercase tracking-[0.18em] text-brand">First-time setup</p>
              <h1 className="mt-2 text-3xl font-semibold tracking-tight text-text-main sm:text-display">Build your music library</h1>
              <p className="mx-auto mt-3 max-w-xl text-text-secondary">
                Choose where ViiB should find your music. You can add the other source later in Settings.
              </p>
            </div>

            <div className="mt-7 grid gap-4 sm:grid-cols-2" role="group" aria-label="Choose a music source">
              <button
                type="button"
                onClick={() => setPhase('local-and-integrations')}
                className="group flex min-h-52 flex-col rounded-xl border border-surface-border bg-surface-1 p-5 text-left transition hover:-translate-y-0.5 hover:border-brand/70 hover:bg-surface-2 hover:shadow-xl hover:shadow-black/20 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand"
              >
                <span className="flex h-11 w-11 items-center justify-center rounded-lg bg-brand/10 text-brand transition group-hover:bg-brand/20"><FolderOpen size={23} /></span>
                <span className="mt-5 text-lg font-semibold text-text-main">Local media</span>
                <span className="mt-1 text-sm leading-6 text-text-secondary">Scan music stored on this computer, an external drive, or a mounted folder.</span>
                <span className="mt-auto pt-5 text-xs font-semibold uppercase tracking-wide text-text-subtle">Folders · optional enhancements · scan</span>
              </button>

              <button
                type="button"
                onClick={() => setPhase('plex')}
                className="group flex min-h-52 flex-col rounded-xl border border-surface-border bg-surface-1 p-5 text-left transition hover:-translate-y-0.5 hover:border-brand/70 hover:bg-surface-2 hover:shadow-xl hover:shadow-black/20 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand"
              >
                <span className="flex h-11 w-11 items-center justify-center rounded-lg bg-brand/10 text-brand transition group-hover:bg-brand/20"><Server size={23} /></span>
                <span className="mt-5 text-lg font-semibold text-text-main">Plex Media Server</span>
                <span className="mt-1 text-sm leading-6 text-text-secondary">Connect a Plex music library and import its catalog before you start listening.</span>
                <span className="mt-auto flex items-center gap-2 pt-5 text-xs font-semibold uppercase tracking-wide text-text-subtle"><Sparkles size={13} className="text-brand" /> Server · library · import</span>
              </button>
            </div>

            <div className="mt-6 flex items-center justify-between border-t border-surface-border pt-5">
              <Button variant="ghost" onClick={onComplete} className="px-0 text-text-subtle hover:text-text-secondary">Set up later</Button>
              <p className="text-right text-xs text-text-subtle">Your media stays under your control.</p>
            </div>
          </>
        ) : (
          <PlexFirstLaunchSetup onBack={() => setPhase('sources')} onComplete={onComplete} />
        )}
      </div>
    </div>
  );
};

export default FirstLaunchDialog;
