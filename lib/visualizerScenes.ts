import type { AudioSettings, VisualizerMode } from '../types';

export type PartyStageLayout = 'artwork' | 'lyrics' | 'visualizer';

export interface VisualizerScene {
    id: string;
    name: string;
    favorite: boolean;
    overlayEnabled: boolean;
    milkdropPreset: string | null;
    layers: Pick<AudioSettings,
        'visualizerMode' | 'visualizerArtworkOpacity' | 'visualizerFullscreenEnabled'
        | 'visualizerFullscreenOpacity' | 'visualizerBackgroundMode'
    >;
}

export const starterVisualizerScenes: VisualizerScene[] = [
    starter('deep-space', 'Deep Space', 'FIREFLY_FIELD', 'STARDUST_HALO', 56, 76),
    starter('electric-night', 'Electric Night', 'ELECTRIC_ARC', 'TUNNEL_WAVEFORM', 42, 68),
    starter('northern-lights', 'Northern Lights', 'WAVE', 'AURORA_RIBBON', 58, 66),
    starter('solar-flare', 'Solar Flare', 'SPECTRUM', 'FLAME_SPECTRUM', 46, 78),
];

function starter(
    id: string,
    name: string,
    visualizerMode: VisualizerMode,
    visualizerBackgroundMode: VisualizerMode,
    visualizerArtworkOpacity: number,
    visualizerFullscreenOpacity: number,
): VisualizerScene {
    return {
        id, name, favorite: true, overlayEnabled: true, milkdropPreset: null,
        layers: {
            visualizerMode,
            visualizerBackgroundMode,
            visualizerArtworkOpacity,
            visualizerFullscreenOpacity,
            visualizerFullscreenEnabled: true,
        },
    };
}

export function nextFavoriteScene(scenes: VisualizerScene[], selectedId: string | null): VisualizerScene | undefined {
    const favorites = scenes.filter(scene => scene.favorite);
    if (favorites.length === 0) return;
    const currentIndex = favorites.findIndex(scene => scene.id === selectedId);
    const next = favorites[(currentIndex + 1) % favorites.length];
    return next?.id === selectedId ? undefined : next;
}
