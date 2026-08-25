import React, { useState } from 'react';
import { ChevronRight, FolderOpen, Music, Server } from 'lucide-react';
import FirstLaunchDialogLegacy from './FirstLaunchDialogLegacy';
import PlexFirstLaunchSetup from './PlexFirstLaunchSetup';
import { Button } from './ui/Button';

interface FirstLaunchDialogProps {
  isOpen: boolean;
  onComplete: () => void;
}

type FirstLaunchPhase = 'sources' | 'local-and-integrations';

/**
 * First-run source selection now treats local folders and Plex as peers. The
 * established local-folder / Spotify / AI / Last.fm flow is preserved intact in
 * FirstLaunchDialogLegacy so adding remote-library onboarding does not destabilize
 * the existing optional integrations.
 */
export const FirstLaunchDialog: React.FC<FirstLaunchDialogProps> = ({ isOpen, onComplete }) => {
  const [phase, setPhase] = useState<FirstLaunchPhase>('sources');

  if (!isOpen) return null;

  if (phase === 'local-and-integrations') {
    return <FirstLaunchDialogLegacy isOpen={isOpen} onComplete={onComplete} />;
  }

  return (
    <div className="fixed inset-0 z-[200] flex items-center justify-center bg-black/90 backdrop-blur-sm animate-in fade-in duration-300">
      <div className="max-h-[90vh] w-full max-w-4xl overflow-y-auto rounded-2xl border border-surface-3 bg-surface-2 p-8 shadow-2xl mx-4">
        <div className="mb-8 text-center">
          <div className="mb-5 flex justify-center">
            <div className="rounded-full border border-surface-border bg-surface-1 p-5">
              <Music size={42} className="text-brand" />
            </div>
          </div>
          <h1 className="text-display font-bold text-text-main">Welcome to ViiB MediaHub</h1>
          <p className="mx-auto mt-3 max-w-2xl text-text-secondary">
            Start with the music you already own. ViiB can use local folders, a Plex Media Server music library, or both.
          </p>
        </div>

        <div className="mb-6 rounded-xl border border-surface-border bg-surface-1 p-5">
          <div className="flex items-start gap-3">
            <div className="rounded-lg bg-brand/10 p-3"><FolderOpen size={22} className="text-brand" /></div>
            <div className="flex-1">
              <h2 className="font-bold text-text-main">Local music folders</h2>
              <p className="mt-1 text-sm text-text-secondary">
                Scan music stored on this computer or on mounted filesystem locations. The next setup screens also configure Spotify metadata, AI DJ, and Last.fm.
              </p>
            </div>
            <Button
              variant="secondary"
              onClick={() => setPhase('local-and-integrations')}
              rightIcon={<ChevronRight size={17} />}
            >
              Set Up Local & Integrations
            </Button>
          </div>
        </div>

        <PlexFirstLaunchSetup />

        <div className="mt-7 flex flex-wrap items-center justify-between gap-3 border-t border-surface-border pt-6">
          <Button
            variant="ghost"
            onClick={onComplete}
            className="text-text-subtle hover:text-text-secondary"
          >
            Skip setup for now
          </Button>
          <div className="flex flex-wrap gap-2">
            <Button
              variant="secondary"
              onClick={() => setPhase('local-and-integrations')}
              leftIcon={<FolderOpen size={17} />}
            >
              Add Local Music / Configure Integrations
            </Button>
            <Button
              variant="primary"
              accent="brand"
              onClick={onComplete}
              leftIcon={<Server size={17} />}
            >
              Finish First-Run Setup
            </Button>
          </div>
        </div>

        <p className="mt-4 text-center text-xs text-text-subtle">
          You can add, change, resynchronize, or remove Plex and local music sources later in Settings.
        </p>
      </div>
    </div>
  );
};

export default FirstLaunchDialog;
