# ViiB MediaHub - Milkdrop/Butterchurn Visualization Integration Plan

## Overview

This document outlines the phased implementation plan for integrating Milkdrop visualizations via the [Butterchurn](https://github.com/jberg/butterchurn) library into ViiB MediaHub. Butterchurn is a WebGL 2 implementation of the classic Winamp Milkdrop visualizer, providing access to hundreds of iconic audio-reactive presets.

---

## Table of Contents

1. [Goals & Objectives](#goals--objectives)
2. [Technical Requirements](#technical-requirements)
3. [Architecture Overview](#architecture-overview)
4. [Phase 1: Foundation](#phase-1-foundation)
5. [Phase 2: Core Integration](#phase-2-core-integration)
6. [Phase 3: Preset Management](#phase-3-preset-management)
7. [Phase 4: Polish & Optimization](#phase-4-polish--optimization)
8. [Phase 5: Advanced Features](#phase-5-advanced-features)
9. [Risk Assessment](#risk-assessment)
10. [Testing Strategy](#testing-strategy)
11. [Timeline Estimates](#timeline-estimates)

---

## Goals & Objectives

### Primary Goals
- [ ] Add Milkdrop-style visualizations as a new option in ViiB MediaHub
- [ ] Maintain existing 21 custom visualizations (no replacement)
- [ ] Provide seamless switching between Canvas 2D and WebGL visualizers
- [ ] Support hundreds of Milkdrop presets with smooth transitions

### Secondary Goals
- [ ] Implement preset favorites/bookmarks
- [ ] Add preset cycling with configurable intervals
- [ ] Support preset search and categorization
- [ ] Enable quality/resolution settings for performance tuning

### Non-Goals
- Creating custom Milkdrop presets (out of scope)
- Supporting Milkdrop 3 (MilkDrop3/projectM features)
- WebGL 1 fallback (WebGL 2 only)

---

## Technical Requirements

### Browser Requirements
| Feature | Required | Notes |
|---------|----------|-------|
| WebGL 2 | ✅ Yes | Core rendering requirement |
| Web Audio API | ✅ Yes | Already used by ViiB |
| OffscreenCanvas | ⚪ Optional | Used internally by Butterchurn |

**Browser Support Matrix:**
- Chrome 56+ ✅
- Firefox 51+ ✅
- Safari 15+ ✅
- Edge 79+ ✅

### Dependencies
```json
{
  "butterchurn": "^3.0.0",
  "butterchurn-presets": "^3.0.0"
}
```

### Bundle Size Impact
| Package | Size (minified + gzip) |
|---------|----------------------|
| butterchurn | ~150 KB |
| butterchurn-presets (base) | ~1.5 MB |
| butterchurn-presets (extra) | ~3 MB |

**Mitigation Strategy:** Lazy loading with dynamic imports

---

## Architecture Overview

### Component Hierarchy
```
App.tsx
└── Layout.tsx
    └── NowPlaying.tsx
        └── AlbumArtVisualizerContainer.tsx (NEW)
            ├── AlbumArtVisualizer.tsx (Canvas 2D - existing)
            └── MilkdropVisualizer.tsx (WebGL - NEW)
```

### Audio Flow Integration
```
[HTML Audio Element]
    │
    ▼
[audioEngine.register()]
    │
    ├──▶ [MediaElementSourceNode]
    │        │
    │        ▼
    │    [InputGain Node]
    │        │
    │        ▼
    │    [10-Band EQ Chain]
    │        │
    │        ├──────────────────────────────────┐
    │        ▼                                  ▼
    │    [AnalyserNode] ◀── Current Viz    [Butterchurn.connectAudio()]
    │        │               reads this         │
    │        ▼                                  │
    │    [MasterGain]                           │
    │        │                                  │
    │        ▼                                  │
    └──▶ [AudioContext.destination] ◀──────────┘
```

### State Management
```typescript
// New fields in playerSlice.ts
interface PlayerSlice {
  // Existing...
  audioSettings: AudioSettings;
  
  // New Milkdrop settings
  milkdropSettings: MilkdropSettings;
  setMilkdropSettings: (settings: Partial<MilkdropSettings>) => void;
}

interface MilkdropSettings {
  enabled: boolean;
  currentPreset: string | null;
  presetCycleEnabled: boolean;
  presetCycleInterval: number; // seconds
  blendDuration: number; // seconds
  quality: 'low' | 'medium' | 'high';
  favoritePresets: string[];
}
```

---

## Phase 1: Foundation

**Duration:** 1-2 days  
**Status:** 🔲 Not Started

### Tasks

#### 1.1 Feature Detection Utility
Create a utility to check Butterchurn support at runtime.

**File:** `lib/milkdropSupport.ts`

```typescript
/**
 * Check if the browser supports Milkdrop visualizations
 * Requires WebGL 2 and Web Audio API
 */
export function isMilkdropSupported(): boolean {
  // Check WebGL 2
  const canvas = document.createElement('canvas');
  let gl: WebGL2RenderingContext | null = null;
  try {
    gl = canvas.getContext('webgl2');
  } catch (e) {
    gl = null;
  }
  
  if (!gl) return false;
  
  // Check Web Audio API
  const audioApiSupported = !!(window.AudioContext || (window as any).webkitAudioContext);
  
  return audioApiSupported;
}

/**
 * Get recommended quality based on device capabilities
 */
export function getRecommendedQuality(): 'low' | 'medium' | 'high' {
  // Check for mobile/low-power devices
  const isMobile = /Android|webOS|iPhone|iPad|iPod/i.test(navigator.userAgent);
  const hasLowMemory = (navigator as any).deviceMemory && (navigator as any).deviceMemory < 4;
  
  if (isMobile || hasLowMemory) return 'low';
  
  // Check GPU capabilities (heuristic)
  const canvas = document.createElement('canvas');
  const gl = canvas.getContext('webgl2');
  if (gl) {
    const debugInfo = gl.getExtension('WEBGL_debug_renderer_info');
    if (debugInfo) {
      const renderer = gl.getParameter(debugInfo.UNMASKED_RENDERER_WEBGL);
      // Intel integrated graphics → medium
      if (/Intel/i.test(renderer)) return 'medium';
    }
  }
  
  return 'high';
}
```

**Acceptance Criteria:**
- [ ] `isMilkdropSupported()` correctly detects WebGL 2 and Audio API
- [ ] Returns `false` in browsers without WebGL 2
- [ ] Quality recommendation considers device capabilities

---

#### 1.2 Update Type Definitions

**File:** `types.ts`

Add new types for Milkdrop integration:

```typescript
// Add to VisualizerMode
export type VisualizerMode = 
  | 'OFF' 
  | 'WAVE' 
  // ... existing modes ...
  | 'WIND_FIELD'
  | 'MILKDROP';        // NEW: Single Milkdrop mode

// New types
export interface MilkdropSettings {
  enabled: boolean;
  currentPreset: string | null;
  presetCycleEnabled: boolean;
  presetCycleInterval: number;
  blendDuration: number;
  quality: 'low' | 'medium' | 'high';
  favoritePresets: string[];
}

export interface MilkdropPresetInfo {
  name: string;
  key: string;
  isFavorite: boolean;
  category?: string;
}
```

**Acceptance Criteria:**
- [ ] TypeScript compiles without errors
- [ ] New types are exported and usable

---

#### 1.3 Audio Engine Enhancement

**File:** `lib/audio.ts`

Add method to expose audio node for Butterchurn connection:

```typescript
class AudioEngine {
  // ... existing code ...
  
  /**
   * Get the master gain node for external audio connections
   * Used by Butterchurn to receive audio for analysis
   */
  getMasterGainNode(): GainNode | null {
    return this.masterGain;
  }
  
  /**
   * Get the audio context for external use
   * Required by Butterchurn visualizer initialization
   */
  getAudioContext(): AudioContext | null {
    return this.context;
  }
}
```

**Acceptance Criteria:**
- [ ] `getMasterGainNode()` returns valid GainNode after init
- [ ] `getAudioContext()` returns valid AudioContext
- [ ] Existing audio functionality unchanged

---

#### 1.4 Package Installation

Add dependencies to package.json:

```bash
npm install butterchurn butterchurn-presets
```

Update package.json types if needed for TypeScript support.

**Acceptance Criteria:**
- [ ] Packages installed successfully
- [ ] No dependency conflicts
- [ ] Build completes without errors

---

## Phase 2: Core Integration

**Duration:** 2-3 days  
**Status:** 🔲 Not Started

### Tasks

#### 2.1 Milkdrop Visualizer Component

**File:** `components/now-playing/MilkdropVisualizer.tsx`

Core component that renders Butterchurn visualizations:

```typescript
/**
 * MilkdropVisualizer - WebGL-based Milkdrop visualization
 * 
 * Uses Butterchurn library to render classic Winamp-style visualizations.
 * Requires WebGL 2 support. Falls back to showing error message if unsupported.
 * 
 * Features:
 * - Lazy loading of Butterchurn library
 * - Preset loading and blending
 * - Quality/resolution control
 * - Responsive canvas sizing
 * - Automatic cleanup on unmount
 */

import React, { useEffect, useRef, useState, useCallback } from 'react';
import { audioEngine } from '../../lib/audio';
import { MilkdropSettings } from '../../types';
import { isMilkdropSupported } from '../../lib/milkdropSupport';

interface MilkdropVisualizerProps {
  settings: MilkdropSettings;
  isActive: boolean;
  onPresetChange?: (presetName: string) => void;
  className?: string;
}

export const MilkdropVisualizer: React.FC<MilkdropVisualizerProps> = ({
  settings,
  isActive,
  onPresetChange,
  className = ''
}) => {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const visualizerRef = useRef<any>(null);
  const animationRef = useRef<number>(0);
  const presetsRef = useRef<Record<string, any>>({});
  const [isLoaded, setIsLoaded] = useState(false);
  const [error, setError] = useState<string | null>(null);
  
  // Quality to resolution mapping
  const qualityMap = {
    low: { width: 640, height: 480 },
    medium: { width: 1280, height: 720 },
    high: { width: 1920, height: 1080 }
  };
  
  // Initialize Butterchurn
  useEffect(() => {
    if (!isActive) return;
    
    const initButterchurn = async () => {
      try {
        if (!isMilkdropSupported()) {
          setError('Milkdrop requires WebGL 2 which is not supported by your browser');
          return;
        }
        
        const canvas = canvasRef.current;
        if (!canvas) return;
        
        // Lazy load Butterchurn
        const butterchurn = await import('butterchurn');
        const butterchurnPresets = await import('butterchurn-presets');
        
        const audioContext = audioEngine.getAudioContext();
        if (!audioContext) {
          setError('Audio context not available');
          return;
        }
        
        const resolution = qualityMap[settings.quality];
        
        // Create visualizer
        visualizerRef.current = butterchurn.default.createVisualizer(
          audioContext,
          canvas,
          {
            width: resolution.width,
            height: resolution.height,
            pixelRatio: window.devicePixelRatio || 1,
            textureRatio: 1,
          }
        );
        
        // Connect audio
        const audioNode = audioEngine.getMasterGainNode();
        if (audioNode) {
          visualizerRef.current.connectAudio(audioNode);
        }
        
        // Load presets
        presetsRef.current = butterchurnPresets.default.getPresets();
        
        // Load initial preset
        if (settings.currentPreset && presetsRef.current[settings.currentPreset]) {
          visualizerRef.current.loadPreset(
            presetsRef.current[settings.currentPreset],
            0
          );
        } else {
          // Load random preset
          const presetKeys = Object.keys(presetsRef.current);
          const randomPreset = presetKeys[Math.floor(Math.random() * presetKeys.length)];
          visualizerRef.current.loadPreset(presetsRef.current[randomPreset], 0);
          onPresetChange?.(randomPreset);
        }
        
        setIsLoaded(true);
      } catch (err) {
        console.error('Failed to initialize Milkdrop:', err);
        setError('Failed to load Milkdrop visualizer');
      }
    };
    
    initButterchurn();
    
    return () => {
      if (visualizerRef.current) {
        visualizerRef.current.loseGLContext?.();
        visualizerRef.current = null;
      }
    };
  }, [isActive]);
  
  // Render loop
  useEffect(() => {
    if (!isLoaded || !isActive) return;
    
    const render = () => {
      animationRef.current = requestAnimationFrame(render);
      visualizerRef.current?.render();
    };
    
    render();
    
    return () => {
      if (animationRef.current) {
        cancelAnimationFrame(animationRef.current);
      }
    };
  }, [isLoaded, isActive]);
  
  // Handle preset changes
  useEffect(() => {
    if (!isLoaded || !settings.currentPreset || !presetsRef.current[settings.currentPreset]) return;
    
    visualizerRef.current?.loadPreset(
      presetsRef.current[settings.currentPreset],
      settings.blendDuration
    );
  }, [settings.currentPreset, isLoaded]);
  
  // Handle resize
  useEffect(() => {
    if (!canvasRef.current || !visualizerRef.current) return;
    
    const resolution = qualityMap[settings.quality];
    visualizerRef.current.setRendererSize(resolution.width, resolution.height);
  }, [settings.quality]);
  
  if (error) {
    return (
      <div className={`flex items-center justify-center ${className}`}>
        <p className="text-red-400 text-sm">{error}</p>
      </div>
    );
  }
  
  return (
    <canvas
      ref={canvasRef}
      className={`w-full h-full ${className}`}
      style={{ display: isActive ? 'block' : 'none' }}
    />
  );
};
```

**Acceptance Criteria:**
- [ ] Component renders WebGL canvas
- [ ] Butterchurn initializes successfully
- [ ] Audio is connected and visualizer responds to music
- [ ] Presets can be loaded and blended
- [ ] Component cleans up on unmount
- [ ] Error state displays appropriate message

---

#### 2.2 Visualizer Container Component

**File:** `components/now-playing/AlbumArtVisualizerContainer.tsx`

Wrapper that switches between Canvas 2D and WebGL visualizers:

```typescript
/**
 * AlbumArtVisualizerContainer - Manages visualization rendering
 * 
 * Routes to appropriate visualizer based on mode:
 * - MILKDROP → MilkdropVisualizer (WebGL)
 * - Other modes → AlbumArtVisualizer (Canvas 2D)
 */

import React from 'react';
import { VisualizerMode, MilkdropSettings } from '../../types';
import { AlbumArtVisualizer } from './AlbumArtVisualizer';
import { MilkdropVisualizer } from './MilkdropVisualizer';

interface Props {
  mode: VisualizerMode;
  milkdropSettings: MilkdropSettings;
  isActive: boolean;
  color?: string;
  accentColor?: string;
  onFadeComplete?: (visible: boolean) => void;
  onPresetChange?: (presetName: string) => void;
  className?: string;
}

export const AlbumArtVisualizerContainer: React.FC<Props> = ({
  mode,
  milkdropSettings,
  isActive,
  color,
  accentColor,
  onFadeComplete,
  onPresetChange,
  className = ''
}) => {
  if (mode === 'MILKDROP') {
    return (
      <MilkdropVisualizer
        settings={milkdropSettings}
        isActive={isActive}
        onPresetChange={onPresetChange}
        className={className}
      />
    );
  }
  
  return (
    <AlbumArtVisualizer
      mode={mode}
      isActive={isActive}
      color={color}
      accentColor={accentColor}
      onFadeComplete={onFadeComplete}
      className={className}
    />
  );
};
```

**Acceptance Criteria:**
- [ ] Container renders correct visualizer based on mode
- [ ] Props are passed correctly to child components
- [ ] Switching between modes works smoothly

---

#### 2.3 State Management Integration

**File:** `slices/playerSlice.ts`

Add Milkdrop settings to player state:

```typescript
// Add to PlayerSlice interface
milkdropSettings: MilkdropSettings;
setMilkdropSettings: (settings: Partial<MilkdropSettings>) => void;
setMilkdropPreset: (preset: string | null) => void;
toggleMilkdropFavorite: (preset: string) => void;

// Default settings
const defaultMilkdropSettings: MilkdropSettings = {
  enabled: false,
  currentPreset: null,
  presetCycleEnabled: true,
  presetCycleInterval: 30, // seconds
  blendDuration: 2.7, // seconds (classic Milkdrop default)
  quality: 'medium',
  favoritePresets: []
};

// Add to slice implementation
milkdropSettings: defaultMilkdropSettings,

setMilkdropSettings: (settings) => set((state) => ({
  milkdropSettings: { ...state.milkdropSettings, ...settings }
})),

setMilkdropPreset: (preset) => set((state) => ({
  milkdropSettings: { ...state.milkdropSettings, currentPreset: preset }
})),

toggleMilkdropFavorite: (preset) => set((state) => {
  const favorites = state.milkdropSettings.favoritePresets;
  const isFavorite = favorites.includes(preset);
  return {
    milkdropSettings: {
      ...state.milkdropSettings,
      favoritePresets: isFavorite
        ? favorites.filter(p => p !== preset)
        : [...favorites, preset]
    }
  };
}),
```

**Acceptance Criteria:**
- [ ] Milkdrop settings persist across sessions
- [ ] Settings can be updated via actions
- [ ] Favorites are properly managed

---

#### 2.4 Update Now Playing Integration

**File:** `components/NowPlaying.tsx`

Update to use new container component and support Milkdrop mode cycling:

```typescript
// Add import
import { AlbumArtVisualizerContainer } from './now-playing/AlbumArtVisualizerContainer';

// Update visualization section
<AlbumArtVisualizerContainer
  mode={audioSettings.visualizerMode}
  milkdropSettings={milkdropSettings}
  isActive={audioSettings.visualizerEnabled && audioSettings.visualizerMode !== 'OFF'}
  color={VIIB_COLOR_VALUES.playbackGreen}
  accentColor={VIIB_COLOR_VALUES.brandPurple}
  onPresetChange={setMilkdropPreset}
  className="absolute inset-0 z-10 pointer-events-none"
/>

// Update cycle function to include MILKDROP
const cycleVisualizerMode = () => {
  const modes: VisualizerMode[] = [
    'OFF', 'WAVE', 'SPECTRUM', /* ... existing ... */, 'WIND_FIELD', 'MILKDROP'
  ];
  const currentIndex = modes.indexOf(audioSettings.visualizerMode);
  const nextIndex = (currentIndex + 1) % modes.length;
  setVisualizerMode(modes[nextIndex]);
};
```

**Acceptance Criteria:**
- [ ] Milkdrop mode accessible via cycle button
- [ ] Visualization renders correctly in Now Playing view
- [ ] Preset changes are tracked in state

---

## Phase 3: Preset Management

**Duration:** 2-3 days  
**Status:** 🔲 Not Started

### Tasks

#### 3.1 Preset Selector Component

**File:** `components/now-playing/MilkdropPresetSelector.tsx`

Modal/panel for browsing and selecting presets:

```typescript
/**
 * MilkdropPresetSelector - Browse and select Milkdrop presets
 * 
 * Features:
 * - Search/filter presets by name
 * - Favorite presets section
 * - Categorized preset list
 * - Preview on hover (optional)
 */

interface Props {
  presets: string[];
  currentPreset: string | null;
  favorites: string[];
  onSelect: (preset: string) => void;
  onToggleFavorite: (preset: string) => void;
  onClose: () => void;
}
```

**Acceptance Criteria:**
- [ ] Displays all available presets
- [ ] Search filters presets in real-time
- [ ] Favorites section at top
- [ ] Current preset highlighted
- [ ] Click selects preset and closes

---

#### 3.2 Preset Auto-Cycling

**File:** `hooks/useMilkdropCycle.ts`

Hook to automatically cycle through presets:

```typescript
/**
 * useMilkdropCycle - Auto-cycle through Milkdrop presets
 * 
 * @param enabled - Whether cycling is enabled
 * @param interval - Seconds between preset changes
 * @param presets - Available preset keys
 * @param onPresetChange - Callback when preset changes
 */
export function useMilkdropCycle(
  enabled: boolean,
  interval: number,
  presets: string[],
  onPresetChange: (preset: string) => void
) {
  useEffect(() => {
    if (!enabled || presets.length === 0) return;
    
    const timer = setInterval(() => {
      const randomIndex = Math.floor(Math.random() * presets.length);
      onPresetChange(presets[randomIndex]);
    }, interval * 1000);
    
    return () => clearInterval(timer);
  }, [enabled, interval, presets, onPresetChange]);
}
```

**Acceptance Criteria:**
- [ ] Presets cycle at configured interval
- [ ] Cycling stops when disabled
- [ ] Random selection avoids recent presets

---

#### 3.3 Settings UI Integration

**File:** `pages/Settings.tsx`

Add Milkdrop settings section:

```typescript
{/* Milkdrop Settings Section */}
{audioSettings.visualizerMode === 'MILKDROP' && (
  <div className="space-y-4 mt-4 p-4 bg-surface-1 rounded-lg">
    <h4 className="text-sm font-medium text-neutral-100">
      Milkdrop Settings
    </h4>
    
    {/* Current Preset */}
    <div className="flex items-center justify-between">
      <span className="text-sm text-neutral-400">Current Preset</span>
      <Button size="sm" onClick={openPresetSelector}>
        {milkdropSettings.currentPreset || 'Select Preset'}
      </Button>
    </div>
    
    {/* Auto-Cycle Toggle */}
    <div className="flex items-center justify-between">
      <span className="text-sm text-neutral-400">Auto-Cycle Presets</span>
      <Switch
        checked={milkdropSettings.presetCycleEnabled}
        onChange={(checked) => setMilkdropSettings({ presetCycleEnabled: checked })}
      />
    </div>
    
    {/* Cycle Interval */}
    {milkdropSettings.presetCycleEnabled && (
      <div className="flex items-center justify-between">
        <span className="text-sm text-neutral-400">Cycle Interval</span>
        <select
          value={milkdropSettings.presetCycleInterval}
          onChange={(e) => setMilkdropSettings({ 
            presetCycleInterval: parseInt(e.target.value) 
          })}
          className="bg-surface-2 text-neutral-100 rounded px-2 py-1"
        >
          <option value={15}>15 seconds</option>
          <option value={30}>30 seconds</option>
          <option value={60}>1 minute</option>
          <option value={120}>2 minutes</option>
        </select>
      </div>
    )}
    
    {/* Quality Setting */}
    <div className="flex items-center justify-between">
      <span className="text-sm text-neutral-400">Quality</span>
      <select
        value={milkdropSettings.quality}
        onChange={(e) => setMilkdropSettings({ 
          quality: e.target.value as 'low' | 'medium' | 'high' 
        })}
        className="bg-surface-2 text-neutral-100 rounded px-2 py-1"
      >
        <option value="low">Low (640x480)</option>
        <option value="medium">Medium (720p)</option>
        <option value="high">High (1080p)</option>
      </select>
    </div>
  </div>
)}
```

**Acceptance Criteria:**
- [ ] Settings section appears when Milkdrop mode active
- [ ] All settings are functional
- [ ] Changes apply immediately
- [ ] Settings persist after restart

---

## Phase 4: Polish & Optimization

**Duration:** 2-3 days  
**Status:** 🔲 Not Started

### Tasks

#### 4.1 Lazy Loading Implementation

Ensure Butterchurn is only loaded when needed:

```typescript
// Dynamic import with loading state
const [butterchurnLoading, setButterchurnLoading] = useState(true);

useEffect(() => {
  const loadButterchurn = async () => {
    setButterchurnLoading(true);
    try {
      await Promise.all([
        import('butterchurn'),
        import('butterchurn-presets')
      ]);
      setButterchurnLoading(false);
    } catch (error) {
      console.error('Failed to load Butterchurn:', error);
    }
  };
  
  if (isActive) {
    loadButterchurn();
  }
}, [isActive]);
```

**Acceptance Criteria:**
- [ ] Butterchurn not loaded until Milkdrop mode selected
- [ ] Loading indicator shown during load
- [ ] Subsequent loads are instant (cached)

---

#### 4.2 Performance Optimization

- [ ] Add frame rate limiter option (30/60 FPS)
- [ ] Pause visualization when window minimized
- [ ] Reduce resolution on high-DPI displays if performance issues
- [ ] Memory cleanup when switching away from Milkdrop

---

#### 4.3 Transition Polish

- [ ] Smooth fade when switching to/from Milkdrop mode
- [ ] Loading skeleton while Butterchurn initializes
- [ ] Graceful degradation if WebGL context lost

---

#### 4.4 Keyboard Shortcuts

Add keyboard shortcuts for Milkdrop control:

| Key | Action |
|-----|--------|
| `M` | Toggle Milkdrop mode |
| `N` | Next preset (random) |
| `B` | Previous preset |
| `F` | Add/remove from favorites |
| `R` | Toggle auto-cycle |

**Acceptance Criteria:**
- [ ] Shortcuts work in Now Playing view
- [ ] Shortcuts documented in UI
- [ ] No conflicts with existing shortcuts

---

## Phase 5: Advanced Features

**Duration:** 3-5 days  
**Status:** 🔲 Not Started

### Tasks

#### 5.1 Preset Categories

Organize presets into categories:
- Abstract
- Cosmic
- Geometric
- Nature
- Trippy
- Favorites

---

#### 5.2 Preset Rating System

Allow users to rate presets 1-5 stars:
- Sort by rating
- Filter by minimum rating
- Exclude low-rated from auto-cycle

---

#### 5.3 Preset Blacklist

Allow users to blacklist presets they don't want:
- Never show in auto-cycle
- Hidden from main list
- Manage blacklist in settings

---

#### 5.4 Extra Presets Package

Option to load `butterchurn-presets-extra` for more presets:
- Toggle in settings
- Download on demand (~3MB)
- Adds 500+ additional presets

---

#### 5.5 Song Title Animation

Use Butterchurn's built-in song title feature:

```typescript
visualizer.launchSongTitleAnim(songTitle);
```

**Acceptance Criteria:**
- [ ] Song title appears when track changes
- [ ] Animation uses Milkdrop-style rendering
- [ ] Configurable on/off in settings

---

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| WebGL 2 not supported | Low | High | Feature detection, graceful fallback to message |
| Performance issues on low-end devices | Medium | Medium | Quality settings, FPS limiter |
| Bundle size too large | Medium | Medium | Lazy loading, code splitting |
| Audio sync issues | Low | High | Use same AudioContext, proper node connection |
| Memory leaks | Medium | Medium | Proper cleanup on unmount, context disposal |
| Preset loading failures | Low | Low | Try-catch, fallback to default preset |

---

## Testing Strategy

### Unit Tests
- [ ] Feature detection utility
- [ ] State management actions
- [ ] Preset selection logic
- [ ] Auto-cycle timer logic

### Integration Tests
- [ ] Audio connection and visualization response
- [ ] Mode switching (Canvas 2D ↔ WebGL)
- [ ] Settings persistence
- [ ] Keyboard shortcuts

### Manual Testing
- [ ] Various browsers (Chrome, Firefox, Safari, Edge)
- [ ] Different devices (desktop, laptop, tablet)
- [ ] Performance profiling with DevTools
- [ ] Memory leak detection over extended use

### Accessibility
- [ ] Reduced motion preference respected
- [ ] Keyboard navigation works
- [ ] Screen reader announces mode changes

---

## Timeline Estimates

| Phase | Duration | Cumulative |
|-------|----------|------------|
| Phase 1: Foundation | 1-2 days | 2 days |
| Phase 2: Core Integration | 2-3 days | 5 days |
| Phase 3: Preset Management | 2-3 days | 8 days |
| Phase 4: Polish & Optimization | 2-3 days | 11 days |
| Phase 5: Advanced Features | 3-5 days | 16 days |

**Total Estimated Time:** 2-3 weeks

---

## Revision History

| Date | Version | Changes |
|------|---------|---------|
| 2026-01-20 | 1.0 | Initial planning document |

---

## References

- [Butterchurn GitHub Repository](https://github.com/jberg/butterchurn)
- [Butterchurn Demo](https://butterchurnviz.com/)
- [Milkdrop Wikipedia](https://en.wikipedia.org/wiki/MilkDrop)
- [WebGL 2 Browser Support](https://caniuse.com/webgl2)
- [ViiB Visualizations Guide](./VISUALIZATIONS_GUIDE.md)
