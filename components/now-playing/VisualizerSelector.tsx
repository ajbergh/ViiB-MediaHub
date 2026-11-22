/**
 * ViiB MediaHub - Visualizer Selector Component
 * 
 * A creative floating panel for selecting visualizer modes for both
 * the album art overlay and the fullscreen background.
 * 
 * Features:
 * - Visual icon grid for each visualizer mode
 * - Separate selection for album art and background
 * - Preview thumbnails (animated icons)
 * - Opacity sliders for each layer
 * - Toggle for enabling/disabling background visualizer
 * 
 * @module VisualizerSelector
 */

import React, { useState } from 'react';
import { useStore } from '../../store';
import { VisualizerMode } from '../../types';
import { Button } from '../ui/Button';
import { VIIB } from '../ui/tokens';
import { VisualizerSceneLibrary } from './VisualizerSceneLibrary';
import { MilkdropPresetSelector } from './MilkdropPresetSelector';
import { useFocusTrap } from '../../hooks/useFocusTrap';
import { 
    X, 
    Waves, 
    BarChart2, 
    Flame, 
    Star, 
    Wind, 
    Zap, 
    Sparkles,
    Circle,
    Layers,
    Eye,
    EyeOff,
    Image,
    MonitorPlay,
    ChevronDown,
    ChevronUp
} from 'lucide-react';

interface VisualizerOption {
    mode: VisualizerMode;
    name: string;
    icon: React.ReactNode;
    description: string;
    color: string;
}

const VISUALIZER_OPTIONS: VisualizerOption[] = [
    { mode: 'OFF', name: 'Off', icon: <EyeOff size={20} />, description: 'No visualization', color: 'text-text-subtle' },
    { mode: 'WAVE', name: 'Wave', icon: <Waves size={20} />, description: 'Braided neon waveform', color: 'text-cyan-400' },
    { mode: 'SPECTRUM', name: 'Spectrum', icon: <BarChart2 size={20} />, description: 'Circular bars', color: 'text-green-400' },
    { mode: 'FLAME_SPECTRUM', name: 'Flames', icon: <Flame size={20} />, description: 'Fire crown with drifting embers', color: 'text-orange-500' },
    { mode: 'STARDUST_HALO', name: 'Stardust', icon: <Star size={20} />, description: 'Glowing orbits in a star field', color: 'text-cyan-300' },
    { mode: 'AURORA_RIBBON', name: 'Aurora', icon: <Sparkles size={20} />, description: 'Layered aurora curtains', color: 'text-emerald-400' },
    { mode: 'ELECTRIC_ARC', name: 'Electric', icon: <Zap size={20} />, description: 'Branching lightning', color: 'text-cyan-400' },
    { mode: 'GRASS_OSCILLOSCOPE', name: 'Grass', icon: <Wind size={20} />, description: 'Backlit grass in rolling waves', color: 'text-lime-400' },
    { mode: 'FIREFLY_FIELD', name: 'Fireflies', icon: <Sparkles size={20} />, description: 'Drifting lights', color: 'text-amber-300' },
    { mode: 'TUNNEL_WAVEFORM', name: 'Tunnel', icon: <Circle size={20} />, description: 'Warping waveform gates', color: 'text-violet-400' },
    { mode: 'WIND_FIELD', name: 'Wind', icon: <Wind size={20} />, description: 'Curving trails of light', color: 'text-sky-300' },
    { mode: 'MILKDROP', name: 'Milkdrop', icon: <Layers size={20} />, description: 'Classic WebGL', color: 'text-purple-400' },
];

interface Props {
    isOpen: boolean;
    onClose: () => void;
}

export const VisualizerSelector: React.FC<Props> = ({ isOpen, onClose }) => {
    const { 
        audioSettings,
        setVisualizerMode,
        setVisualizerBackgroundMode,
        setVisualizerArtworkOpacity,
        setVisualizerFullscreenEnabled,
        setVisualizerFullscreenOpacity,
        artworkOverlayEnabled,
        setArtworkOverlayEnabled,
        milkdropSettings,
        milkdropPresetKeys,
        setMilkdropPreset,
        toggleMilkdropFavorite,
    } = useStore();
    
    const [expandedSection, setExpandedSection] = useState<'artwork' | 'background' | null>('artwork');
    const [view, setView] = useState<'scenes' | 'layers'>('scenes');
    const [showMilkdropPresets, setShowMilkdropPresets] = useState(false);
    const dialogRef = useFocusTrap<HTMLDivElement>(isOpen && !showMilkdropPresets, onClose);
    
    if (!isOpen) return null;
    
    const currentArtworkOption = VISUALIZER_OPTIONS.find(o => o.mode === audioSettings.visualizerMode) || VISUALIZER_OPTIONS[0];
    const currentBackgroundOption = VISUALIZER_OPTIONS.find(o => o.mode === audioSettings.visualizerBackgroundMode) || VISUALIZER_OPTIONS[0];
    
    const renderModeGrid = (
        selectedMode: VisualizerMode, 
        onSelect: (mode: VisualizerMode) => void
    ) => (
        <div className="grid grid-cols-3 sm:grid-cols-4 gap-2 p-2">
            {VISUALIZER_OPTIONS.map(option => (
                <button
                    key={option.mode}
                    onClick={() => onSelect(option.mode)}
                    aria-pressed={selectedMode === option.mode}
                    className={`
                        flex flex-col items-center justify-center p-3 rounded-lg transition-all duration-200
                        ${selectedMode === option.mode 
                            ? 'bg-brand/20 ring-2 ring-brand shadow-lg shadow-brand/20' 
                            : 'bg-surface-2/60 hover:bg-surface-2 hover:scale-105'
                        }
                    `}
                    title={option.description}
                >
                    <span className={`${option.color} ${selectedMode === option.mode ? 'scale-110' : ''} transition-transform`}>
                        {option.icon}
                    </span>
                    <span className="text-xs mt-1 text-text-secondary truncate w-full text-center">
                        {option.name}
                    </span>
                </button>
            ))}
        </div>
    );
    
    return (
        <div className={`fixed inset-0 ${VIIB.zIndex.overlayPanel} flex items-center justify-center bg-black/60 backdrop-blur-sm animate-in fade-in duration-200 motion-reduce:animate-none`} onClick={onClose}>
            <div 
                role="dialog"
                ref={dialogRef}
                tabIndex={-1}
                inert={showMilkdropPresets}
                aria-modal="true"
                aria-label="Visualizer Layers"
                className="bg-surface-1/95 backdrop-blur-xl rounded-2xl shadow-2xl border border-surface-3/50 max-w-xl w-full mx-4 max-h-[86dvh] overflow-hidden flex flex-col animate-in zoom-in-95 duration-200 motion-reduce:animate-none"
                onClick={e => e.stopPropagation()}
            >
                {/* Header */}
                <div className="flex items-center justify-between p-4 border-b border-surface-3/50">
                    <div className="flex items-center gap-3">
                        <div className="p-2 bg-brand/20 rounded-lg">
                            <Layers className="text-brand" size={20} />
                        </div>
                        <div>
                            <h2 className="text-lg font-bold">Visualizer Layers</h2>
                        </div>
                    </div>
                    <Button
                        onClick={onClose}
                        variant="ghost"
                        className="rounded-full p-2 hover:bg-surface-2"
                        aria-label="Close visualizer layers"
                    >
                        <X size={20} />
                    </Button>
                </div>
                
                <div className="grid grid-cols-2 gap-1.5 border-b border-surface-border px-4 py-2" role="group" aria-label="Visualizer settings">
                    <Button variant={view === 'scenes' ? 'secondary' : 'ghost'} aria-pressed={view === 'scenes'} onClick={() => setView('scenes')}>Scenes</Button>
                    <Button variant={view === 'layers' ? 'secondary' : 'ghost'} aria-pressed={view === 'layers'} onClick={() => setView('layers')}>Layers</Button>
                </div>
                {view === 'scenes' ? (
                    <div className="min-h-0 overflow-y-auto">
                        <VisualizerSceneLibrary
                            onApply={onClose}
                            modeLabel={mode => VISUALIZER_OPTIONS.find(option => option.mode === mode)?.name ?? mode}
                        />
                    </div>
                ) : (
                    <div className="min-h-0 flex-1 overflow-y-auto">
                        {/* Album Art Visualizer Section */}
                        <div className="border-b border-surface-3/30">
                            <button
                                onClick={() => setExpandedSection(expandedSection === 'artwork' ? null : 'artwork')}
                                className="w-full flex items-center justify-between p-4 hover:bg-surface-2/30 transition-colors"
                            >
                                <div className="flex items-center gap-3">
                                    <div className="p-2 bg-accent-green/20 rounded-lg">
                                        <Image className="text-accent-green" size={18} />
                                    </div>
                                    <div className="text-left">
                                        <div className="font-medium flex items-center gap-2">
                                            Album Art Overlay
                                            <span className={`text-xs px-2 py-0.5 rounded-full ${currentArtworkOption.color} bg-surface-2`}>
                                                {currentArtworkOption.name}
                                            </span>
                                        </div>
                                        <p className="text-xs text-text-subtle">Visualizer over album artwork</p>
                                    </div>
                                </div>
                                {expandedSection === 'artwork' ? <ChevronUp size={20} /> : <ChevronDown size={20} />}
                            </button>
                        
                            {expandedSection === 'artwork' && (
                                <div className="px-4 pb-4 space-y-4 animate-in slide-in-from-top-2 duration-200">
                                    <label className="flex items-center justify-between gap-3 px-2 text-sm text-text-secondary">
                                        Enable artwork overlay
                                        <input type="checkbox" checked={artworkOverlayEnabled} onChange={event => setArtworkOverlayEnabled(event.target.checked)} className="size-5 accent-brand" />
                                    </label>
                                    {renderModeGrid(audioSettings.visualizerMode, setVisualizerMode)}
                                
                                    {/* Artwork Opacity Slider */}
                                    <div className="px-2 space-y-2">
                                        <div className="flex items-center justify-between text-sm">
                                            <span className="text-text-secondary">Artwork Opacity</span>
                                            <span className="text-text-subtle">{audioSettings.visualizerArtworkOpacity}%</span>
                                        </div>
                                        <input
                                            type="range"
                                            min="0"
                                            max="100"
                                            value={audioSettings.visualizerArtworkOpacity}
                                            aria-label="Artwork opacity"
                                            onChange={e => setVisualizerArtworkOpacity(parseInt(e.target.value))}
                                            className="w-full h-7 accent-accent-green cursor-pointer"
                                        />
                                    </div>
                                </div>
                            )}
                        </div>
                    
                        {/* Background Visualizer Section */}
                        <div>
                            <button
                                onClick={() => setExpandedSection(expandedSection === 'background' ? null : 'background')}
                                className="w-full flex items-center justify-between p-4 hover:bg-surface-2/30 transition-colors"
                            >
                                <div className="flex items-center gap-3">
                                    <div className={`p-2 rounded-lg ${audioSettings.visualizerFullscreenEnabled ? 'bg-brand/20' : 'bg-surface-2'}`}>
                                        <MonitorPlay className={audioSettings.visualizerFullscreenEnabled ? 'text-brand' : 'text-text-subtle'} size={18} />
                                    </div>
                                    <div className="text-left">
                                        <div className="font-medium flex items-center gap-2">
                                            Background Layer
                                            {audioSettings.visualizerFullscreenEnabled ? (
                                                <span className={`text-xs px-2 py-0.5 rounded-full ${currentBackgroundOption.color} bg-surface-2`}>
                                                    {currentBackgroundOption.name}
                                                </span>
                                            ) : (
                                                <span className="text-xs px-2 py-0.5 rounded-full text-text-subtle bg-surface-2">
                                                    Disabled
                                                </span>
                                            )}
                                        </div>
                                        <p className="text-xs text-text-subtle">Fullscreen behind all UI</p>
                                    </div>
                                </div>
                                {expandedSection === 'background' ? <ChevronUp size={20} /> : <ChevronDown size={20} />}
                            </button>
                        
                            {expandedSection === 'background' && (
                                <div className="px-4 pb-4 space-y-4 animate-in slide-in-from-top-2 duration-200">
                                    {/* Enable Toggle */}
                                    <div className="flex items-center justify-between px-2">
                                        <span className="text-sm text-text-secondary">Enable Background</span>
                                        <button
                                            onClick={() => setVisualizerFullscreenEnabled(!audioSettings.visualizerFullscreenEnabled)}
                                            aria-label="Enable background layer"
                                            role="switch"
                                            aria-checked={audioSettings.visualizerFullscreenEnabled}
                                            className={`relative w-12 h-6 rounded-full transition-colors ${
                                                audioSettings.visualizerFullscreenEnabled ? 'bg-brand' : 'bg-surface-3'
                                            }`}
                                        >
                                            <span className={`absolute top-1 w-4 h-4 rounded-full bg-white shadow transition-all ${
                                                audioSettings.visualizerFullscreenEnabled ? 'left-7' : 'left-1'
                                            }`} />
                                        </button>
                                    </div>
                                
                                    {audioSettings.visualizerFullscreenEnabled && (
                                        <>
                                            {renderModeGrid(audioSettings.visualizerBackgroundMode ?? 'OFF', setVisualizerBackgroundMode)}
                                        
                                            {/* Background Opacity Slider */}
                                            <div className="px-2 space-y-2">
                                                <div className="flex items-center justify-between text-sm">
                                                    <span className="text-text-secondary">Background Opacity</span>
                                                    <span className="text-text-subtle">{audioSettings.visualizerFullscreenOpacity}%</span>
                                                </div>
                                                <input
                                                    type="range"
                                                    min="0"
                                                    max="100"
                                                    value={audioSettings.visualizerFullscreenOpacity}
                                                    aria-label="Background opacity"
                                                    onChange={e => setVisualizerFullscreenOpacity(parseInt(e.target.value))}
                                                    className="w-full h-7 accent-brand cursor-pointer"
                                                />
                                            </div>
                                        </>
                                    )}
                                </div>
                            )}
                        </div>
                        {(audioSettings.visualizerMode === 'MILKDROP' || audioSettings.visualizerBackgroundMode === 'MILKDROP') && (
                            <div className="flex flex-wrap items-center justify-between gap-2 border-t border-surface-border p-4">
                                <span className="min-w-0 truncate text-sm text-text-secondary">{milkdropSettings.currentPreset || 'Random Milkdrop preset'}</span>
                                <Button onClick={() => setShowMilkdropPresets(true)} disabled={milkdropPresetKeys.length === 0}>Choose Milkdrop preset</Button>
                            </div>
                        )}
                    </div>
                )}
            </div>
            {showMilkdropPresets && (
                <div onClick={event => event.stopPropagation()}>
                    <MilkdropPresetSelector
                        presets={milkdropPresetKeys}
                        currentPreset={milkdropSettings.currentPreset}
                        favorites={milkdropSettings.favoritePresets}
                        onSelect={setMilkdropPreset}
                        onToggleFavorite={toggleMilkdropFavorite}
                        onClose={() => setShowMilkdropPresets(false)}
                    />
                </div>
            )}
        </div>
    );
};
