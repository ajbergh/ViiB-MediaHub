import { useState } from 'react';
import { ArrowUpRight, Plus, Star, Trash2 } from 'lucide-react';
import { useStore } from '../../store';
import type { VisualizerMode } from '../../types';
import { Button } from '../ui/Button';

const sceneColors: Record<string, string> = {
    'deep-space': 'from-accent-purple/55 to-accent-blue/15',
    'electric-night': 'from-accent-blue/55 to-accent-crimson/20',
    'northern-lights': 'from-accent-green/40 to-accent-blue/25',
    'solar-flare': 'from-accent-orange/55 to-accent-crimson/20',
};

interface Props {
    onApply: () => void;
    modeLabel: (mode: VisualizerMode) => string;
}

export function VisualizerSceneLibrary({ onApply, modeLabel }: Props) {
    const scenes = useStore(state => state.visualizerScenes);
    const activeId = useStore(state => state.activeVisualizerSceneId);
    const apply = useStore(state => state.applyVisualizerScene);
    const save = useStore(state => state.saveVisualizerScene);
    const toggleFavorite = useStore(state => state.toggleVisualizerSceneFavorite);
    const remove = useStore(state => state.removeVisualizerScene);
    const rotationEnabled = useStore(state => state.partySceneRotationEnabled);
    const setRotationEnabled = useStore(state => state.setPartySceneRotationEnabled);
    const [name, setName] = useState('');
    const [confirmRemoval, setConfirmRemoval] = useState<string | null>(null);
    const favoriteCount = scenes.filter(scene => scene.favorite).length;

    return (
        <div className="space-y-5 p-4 sm:p-5">
            <div className="grid gap-3 sm:grid-cols-2">
                {scenes.map(scene => {
                    const active = activeId === scene.id;
                    return (
                        <article key={scene.id} className={`relative flex min-w-0 flex-col overflow-hidden rounded-2xl border bg-surface-0/55 ${active ? 'border-brand' : 'border-surface-border'}`}>
                            <button
                                type="button"
                                onClick={() => { apply(scene.id); onApply(); }}
                                aria-label={`Apply ${scene.name}`}
                                className="group min-h-[104px] w-full text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-brand"
                            >
                                <span aria-hidden="true" className={`pointer-events-none absolute inset-x-0 top-0 h-28 bg-gradient-to-br ${sceneColors[scene.id] ?? 'from-brand/45 to-accent-green/10'}`} />
                                <span className="relative flex items-start justify-between gap-2 p-3.5">
                                    <span className="min-w-0">
                                        <span className="block truncate text-base font-bold">{scene.name}</span>
                                        <span className="mt-1.5 block text-xs leading-relaxed text-text-secondary">
                                            {scene.layers.visualizerFullscreenEnabled ? modeLabel(scene.layers.visualizerBackgroundMode) : 'Background off'}
                                            {' / '}
                                            {scene.overlayEnabled ? modeLabel(scene.layers.visualizerMode) : 'Overlay off'}
                                        </span>
                                    </span>
                                    <ArrowUpRight size={16} className="shrink-0 text-text-secondary group-hover:text-text-main" aria-hidden="true" />
                                </span>
                            </button>
                            <div className="relative flex min-h-10 items-center justify-end gap-1 border-t border-surface-border/60 px-2">
                                {confirmRemoval === scene.id ? (
                                    <>
                                        <Button size="sm" variant="ghost" onClick={() => setConfirmRemoval(null)}>Cancel</Button>
                                        <Button size="sm" onClick={() => { remove(scene.id); setConfirmRemoval(null); }}>Remove scene</Button>
                                    </>
                                ) : (
                                    <>
                                        <Button size="icon" variant="ghost" className={scene.favorite ? 'text-accent-orange' : ''} aria-label={`Favorite ${scene.name}`} aria-pressed={scene.favorite} onClick={() => toggleFavorite(scene.id)}>
                                            <Star size={16} className={scene.favorite ? 'fill-current' : ''} />
                                        </Button>
                                        <Button size="icon" variant="ghost" aria-label={`Remove ${scene.name}`} onClick={() => setConfirmRemoval(scene.id)}>
                                            <Trash2 size={15} />
                                        </Button>
                                    </>
                                )}
                            </div>
                        </article>
                    );
                })}
            </div>
            {scenes.length === 0 && <p className="text-sm text-text-secondary">Save your current layers to create your first scene.</p>}
            <form className="flex flex-wrap items-center gap-2" onSubmit={event => {
                event.preventDefault();
                if (!name.trim()) return;
                save(name);
                setName('');
            }}>
                <input
                    aria-label="Scene name"
                    placeholder="Name the current scene"
                    value={name}
                    maxLength={48}
                    onChange={event => setName(event.target.value)}
                    className="min-w-0 flex-1 rounded-xl border border-surface-border bg-surface-0 px-3 py-2 text-sm text-text-main focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand"
                />
                <Button type="submit" disabled={!name.trim()} leftIcon={<Plus size={16} />}>Save scene</Button>
            </form>
            <label className="flex items-center justify-between gap-4 rounded-xl border border-surface-border bg-surface-0/50 p-3">
                <span>
                    <span className="block text-sm font-semibold">Rotate favorites in Party Mode</span>
                    <span className="mt-0.5 block text-xs text-text-secondary">{favoriteCount} {favoriteCount === 1 ? 'favorite' : 'favorites'} · switches between tracks</span>
                </span>
                <input type="checkbox" checked={rotationEnabled} onChange={event => setRotationEnabled(event.target.checked)} className="size-5 shrink-0 accent-brand" />
            </label>
        </div>
    );
}
