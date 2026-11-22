import { useEffect, useState } from 'react';
import type { ReactNode } from 'react';
import { Disc, Lock, Mic2, MonitorPlay, Pin, Unlock } from 'lucide-react';
import { useStore } from '../../store';
import { Button } from '../ui/Button';

const stageLayouts = [
    { id: 'artwork', name: 'Artwork', icon: Disc },
    { id: 'lyrics', name: 'Artwork + Lyrics', icon: Mic2 },
    { id: 'visualizer', name: 'Visualizer', icon: MonitorPlay },
] as const;

export function PartyStageControls() {
    const layout = useStore(state => state.partyStageLayout);
    const setLayout = useStore(state => state.setPartyStageLayout);
    const trackInfoPinned = useStore(state => state.partyTrackInfoPinned);
    const setTrackInfoPinned = useStore(state => state.setPartyTrackInfoPinned);
    const locked = useStore(state => state.partySceneLocked);
    const setLocked = useStore(state => state.setPartySceneLocked);
    const rotating = useStore(state => state.partySceneRotationEnabled);
    const sceneName = useStore(state => state.visualizerScenes.find(scene => scene.id === state.activeVisualizerSceneId)?.name);
    const lockLabel = locked ? 'Unlock scene' : 'Lock scene';

    return (
        <div className="mx-auto flex max-w-full flex-wrap items-center justify-center gap-2">
            <div role="group" aria-label="Party stage layout" className="inline-flex max-w-full flex-wrap items-center justify-center gap-1 rounded-2xl border border-surface-border bg-surface-1/70 p-1 shadow-lg backdrop-blur-md">
                {stageLayouts.map(({ id, name, icon: Icon }) => (
                    <Button key={id} size="sm" className="rounded-xl whitespace-nowrap" variant={layout === id ? 'secondary' : 'ghost'} aria-pressed={layout === id} onClick={() => setLayout(id)}>
                        <Icon size={14} aria-hidden="true" />
                        {name}
                    </Button>
                ))}
            </div>
            <Button
                size="sm"
                variant={trackInfoPinned ? 'secondary' : 'ghost'}
                aria-label="Keep track info visible"
                aria-pressed={trackInfoPinned}
                title="Keep track info visible"
                className="min-h-8 rounded-xl border border-surface-border"
                onClick={() => setTrackInfoPinned(!trackInfoPinned)}
            >
                <Pin size={14} className={trackInfoPinned ? 'fill-current text-brand' : ''} aria-hidden="true" />
                <span className="hidden sm:inline">Track info</span>
            </Button>
            {rotating && (
                <Button
                    size="sm"
                    aria-label={lockLabel}
                    aria-pressed={locked}
                    title={lockLabel}
                    className={`min-w-0 rounded-xl border border-surface-border ${locked ? 'text-accent-green' : ''}`}
                    onClick={() => setLocked(!locked)}
                    leftIcon={locked ? <Lock size={14} /> : <Unlock size={14} />}
                >
                    <span className="max-w-32 truncate">{sceneName ?? lockLabel}</span>
                </Button>
            )}
        </div>
    );
}

export function PartyTrackIntro({ children, className }: { children: ReactNode; className: string }) {
    const pinned = useStore(state => state.partyTrackInfoPinned);
    const [showIntro, setShowIntro] = useState(true);
    useEffect(() => {
        const timeout = window.setTimeout(() => setShowIntro(false), 4600);
        return () => window.clearTimeout(timeout);
    }, []);

    const visible = pinned || showIntro;
    return (
        <div
            role="status"
            inert={!visible}
            className={`transition-opacity duration-500 motion-reduce:transition-none ${className} ${visible ? 'opacity-100' : 'opacity-0'} ${!pinned && showIntro ? 'motion-safe:animate-fade-in' : ''}`}
        >
            {children}
        </div>
    );
}
