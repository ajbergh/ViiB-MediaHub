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
    { mode: 'WAVE', name: 'Wave', icon: <Waves size={20} />, description: 'Smooth waveform', color: 'text-cyan-400' },
    { mode: 'SPECTRUM', name: 'Spectrum', icon: <BarChart2 size={20} />, description: 'Circular bars', color: 'text-green-400' },
    { mode: 'FLAME_SPECTRUM', name: 'Flames', icon: <Flame size={20} />, description: 'Rising flames', color: 'text-orange-500' },
    { mode: 'STARDUST_HALO', name: 'Stardust', icon: <Star size={20} />, description: 'Particle ring', color: 'text-cyan-300' },
    { mode: 'AURORA_RIBBON', name: 'Aurora', icon: <Sparkles size={20} />, description: 'Flowing ribbon', color: 'text-emerald-400' },
    { mode: 'ELECTRIC_ARC', name: 'Electric', icon: <Zap size={20} />, description: 'Lightning arcs', color: 'text-cyan-400' },
    { mode: 'GRASS_OSCILLOSCOPE', name: 'Grass', icon: <Wind size={20} />, description: 'Swaying blades', color: 'text-lime-400' },
    { mode: 'FIREFLY_FIELD', name: 'Fireflies', icon: <Sparkles size={20} />, description: 'Drifting lights', color: 'text-amber-300' },
    { mode: 'TUNNEL_WAVEFORM', name: 'Tunnel', icon: <Circle size={20} />, description: '3D ring tunnel', color: 'text-violet-400' },
    { mode: 'WIND_FIELD', name: 'Wind', icon: <Wind size={20} />, description: 'Flowing particles', color: 'text-sky-300' },
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
        setVisualizerFullscreenOpacity
    } = useStore();
    
    const [expandedSection, setExpandedSection] = useState<'artwork' | 'background' | null>('artwork');
    
    if (!isOpen) return null;
    
    const currentArtworkOption = VISUALIZER_OPTIONS.find(o => o.mode === audioSettings.visualizerMode) || VISUALIZER_OPTIONS[0];
    const currentBackgroundOption = VISUALIZER_OPTIONS.find(o => o.mode === audioSettings.visualizerBackgroundMode) || VISUALIZER_OPTIONS[0];
    
    const renderModeGrid = (
        selectedMode: VisualizerMode, 
        onSelect: (mode: VisualizerMode) => void
    ) => (
        <div className="grid grid-cols-4 gap-2 p-2">
            {VISUALIZER_OPTIONS.map(option => (
                <button
                    key={option.mode}
                    onClick={() => onSelect(option.mode)}
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
        <div className="fixed inset-0 z-[110] flex items-center justify-center bg-black/60 backdrop-blur-sm animate-in fade-in duration-200">
            <div 
                className="bg-surface-1/95 backdrop-blur-xl rounded-2xl shadow-2xl border border-surface-3/50 max-w-md w-full mx-4 max-h-[80vh] overflow-hidden flex flex-col animate-in zoom-in-95 duration-200"
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
                            <p className="text-xs text-text-subtle">Mix and match visualizations</p>
                        </div>
                    </div>
                    <Button
                        onClick={onClose}
                        variant="ghost"
                        className="rounded-full p-2 hover:bg-surface-2"
                    >
                        <X size={20} />
                    </Button>
                </div>
                
                {/* Content */}
                <div className="flex-1 overflow-y-auto">
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
                                        onChange={e => setVisualizerArtworkOpacity(parseInt(e.target.value))}
                                        className="w-full accent-accent-green"
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
                                                onChange={e => setVisualizerFullscreenOpacity(parseInt(e.target.value))}
                                                className="w-full accent-brand"
                                            />
                                        </div>
                                    </>
                                )}
                            </div>
                        )}
                    </div>
                </div>
                
                {/* Footer */}
                <div className="p-4 border-t border-surface-3/50 bg-surface-0/50">
                    <p className="text-xs text-text-subtle text-center">
                        Click album art to toggle overlay • Changes save automatically
                    </p>
                </div>
            </div>
        </div>
    );
};
