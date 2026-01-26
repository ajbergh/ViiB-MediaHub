# WebGL Visualizer Refactoring Plan

## Executive Summary

This document outlines a comprehensive plan to refactor ViiB MediaHub's remaining 10 Canvas 2D visualizations to optimized WebGL implementations. The goal is to achieve **60+ FPS on mid-range hardware** while maintaining or improving visual quality.

**Status**: 🚧 In Progress  
**Last Updated**: 2025-01-27  
**Phase**: Phase 6 - Integration & Polish

---

## Current State Analysis

### Existing Implementation

The current visualizer is implemented in `components/now-playing/AlbumArtVisualizer.tsx` (1331 lines) using Canvas 2D API. It supports 10 visualization modes plus OFF and MILKDROP (WebGL via Butterchurn).

**Available VisualizerMode Types** (from `types.ts`):
- `OFF` - No visualization
- `WAVE` - Smooth waveform
- `SPECTRUM` - Circular frequency bars
- `FLAME_SPECTRUM` - Fire particle system
- `STARDUST_HALO` - Particle halo with trails
- `AURORA_RIBBON` - Northern lights effect
- `ELECTRIC_ARC` - TRON-style lightning
- `GRASS_OSCILLOSCOPE` - Swaying grass blades
- `FIREFLY_FIELD` - Drifting firefly particles
- `TUNNEL_WAVEFORM` - 3D ring tunnel
- `WIND_FIELD` - Flowing wind particles
- `MILKDROP` - Butterchurn WebGL (already GPU-accelerated)

### Remaining Canvas 2D Visualizations (10 total)

| Mode | Lines of Code | Performance Issues | Complexity | Priority |
|------|---------------|-------------------|------------|----------|
| **WAVE** | ~50 | shadowBlur expensive | Low | P1 |
| **SPECTRUM** | ~70 | shadowBlur + radial gradient per bar | Medium | P1 |
| **FLAME_SPECTRUM** | ~80 | Particle system + composite operations (`lighter`) | High | P3 |
| **STARDUST_HALO** | ~100 | 300+ particles + trails + radial gradients | High | P3 |
| **AURORA_RIBBON** | ~80 | Multi-layer rendering + gradients | Medium | P2 |
| **ELECTRIC_ARC** | ~90 | Shadow blur + random jitter per segment | Medium | P2 |
| **GRASS_OSCILLOSCOPE** | ~60 | 80 bezier curves + gradients | Medium | P2 |
| **FIREFLY_FIELD** | ~70 | 40 particles + radial gradients | Medium | P1 |
| **TUNNEL_WAVEFORM** | ~100 | 768 points + perspective projection | High | P4 |
| **WIND_FIELD** | ~70 | 100 particles + radial gradients | Medium | P3 |

### Supporting Files

| File | Purpose | Lines |
|------|---------|-------|
| `AlbumArtVisualizer.tsx` | Main visualizer component with all Canvas 2D modes | 1331 |
| `CanvasPostProcessor.ts` | Offscreen bloom/trail rendering pipeline | 167 |
| `SpriteGenerator.ts` | Procedural sprite generation (glow, flare, ring, cloud) | 130 |

### Current Performance Bottlenecks

1. **Canvas 2D `shadowBlur`**: Extremely expensive, causes ~60% of frame time in SPECTRUM/WAVE
2. **Radial Gradients**: Each `createRadialGradient()` call is costly, especially per-particle
3. **Composite Operations**: `globalCompositeOperation = 'lighter'` forces readback
4. **CPU-bound Math**: Particle physics, trigonometry, bezier calculations all on main thread
5. **State Changes**: Frequent `fillStyle`, `strokeStyle`, `lineWidth` changes

### Profiling Estimates (1080p, 60Hz target)

| Mode | Current FPS | Target FPS | Bottleneck |
|------|-------------|------------|------------|
| WAVE | 45-55 | 60+ | shadowBlur |
| SPECTRUM | 35-45 | 60+ | shadowBlur + gradients |
| FLAME_SPECTRUM | 25-40 | 60+ | Particle count + blending |
| STARDUST_HALO | 20-35 | 60+ | Trail rendering + particles |
| AURORA_RIBBON | 40-55 | 60+ | Multi-layer gradients |
| ELECTRIC_ARC | 35-50 | 60+ | Random jitter + shadow |
| GRASS_OSCILLOSCOPE | 40-50 | 60+ | Bezier curves |
| FIREFLY_FIELD | 45-55 | 60+ | Radial gradients |
| TUNNEL_WAVEFORM | 20-35 | 60+ | Perspective math + point count |
| WIND_FIELD | 40-55 | 60+ | Particle gradients |

---

## Performance Optimization Pass (v2) ✅ Complete

### GPU Utilization Issues Identified and Fixed

After initial implementation, profiling revealed high GPU utilization on several visualizations. The following optimizations were applied:

#### 1. Loop Count Reductions

| Shader | Before | After | Reduction |
|--------|--------|-------|-----------|
| **STARDUST_HALO** | 30 waves × 15 particles = 450 | 12 waves × 8 particles = 96 | **4.7x fewer iterations** |
| **WIND_FIELD** | 100 particles + 5 flow lines = 105 | 50 particles = 50 | **2.1x fewer iterations** |
| **FLAME_SPECTRUM** | 32 columns × 3 FBM octaves | 16 columns × 2 FBM octaves | **3x fewer iterations** |
| **TUNNEL_WAVEFORM** | 24 rings | 16 rings | **1.5x fewer iterations** |
| **ELECTRIC_ARC** | 8 arcs × 8 segments = 64 | 5 arcs × 6 segments = 30 | **2.1x fewer iterations** |

#### 2. Early Exit Optimizations

All heavy shaders now include early exit conditions:
- **Distance-based culling**: Skip particles/elements far from current pixel
- **Bounding box tests**: Skip entire arcs/flames outside visible area
- **Alpha threshold**: Skip faded elements (life < threshold)
- **Region exclusion**: Exit early for pixels in empty regions (e.g., top 30% for flames)

#### 3. Math Simplifications

- Replaced `exp(-x*x)` glow with `max(0, 1-x/r)²` for smoother falloff
- Removed trail calculations from STARDUST (minimal visual impact)
- Simplified HSV→RGB conversion (dedicated function instead of inline)
- Hoisted trigonometry outside loops (`cos`/`sin` precomputed)
- Used distance squared comparisons to avoid `sqrt()` in inner loops

#### 4. Renderer-Level Optimizations

- **DPR Capping**: Maximum device pixel ratio limited to 1.5 to prevent excessive pixel counts on HiDPI displays
- **Texture lookups**: Hoisted outside inner loops where possible
- **Constant precomputation**: Time-based seeds and constants computed once per frame

### Expected Performance Gains

| Shader | Previous WebGL | Optimized v2 | Improvement |
|--------|---------------|--------------|-------------|
| STARDUST_HALO | 25-35 FPS | 55-60+ FPS | ~2x |
| FLAME_SPECTRUM | 30-40 FPS | 55-60+ FPS | ~1.7x |
| TUNNEL_WAVEFORM | 35-45 FPS | 55-60+ FPS | ~1.5x |
| ELECTRIC_ARC | 40-50 FPS | 55-60+ FPS | ~1.3x |
| WIND_FIELD | 40-50 FPS | 55-60+ FPS | ~1.3x |

---

## Implementation Progress

### Phase 1: Core Infrastructure ✅ Complete
- [x] `WebGLVisualizerRenderer.ts` - Main renderer class (~450 LOC)
- [x] `ShaderProgram.ts` - Shader compilation/linking utilities (~200 LOC)
- [x] `AudioTextureManager.ts` - Audio data → texture upload (~180 LOC)
- [x] `SpriteAtlas.ts` - Shared texture atlas (~200 LOC)
- [x] Common shaders (`common.ts`, `noise.ts`, `sdf.ts`, `audio.ts`)
- [x] `shaders/index.ts` - Centralized shader exports

### Phase 2: Simple Visualizations ✅ Complete
- [x] WAVE - SDF waveform with analytical glow (`wave.ts`)
- [x] SPECTRUM - Circular bars with SDF glow (`spectrum.ts`)
- [x] FIREFLY_FIELD - Procedural particles (`firefly.ts`)

### Phase 3: Medium Complexity ✅ Complete
- [x] AURORA_RIBBON - Procedural noise ribbon (`aurora.ts`)
- [x] ELECTRIC_ARC - Procedural lightning (`electric.ts`) - **Optimized v2**
- [x] GRASS_OSCILLOSCOPE - Bezier blade rendering (`grass.ts`)

### Phase 4: Particle Systems ✅ Complete
- [x] FLAME_SPECTRUM - Procedural fire particles (`flame.ts`) - **Optimized v2**
- [x] STARDUST_HALO - Particle trails + ring (`stardust.ts`) - **Optimized v2**
- [x] WIND_FIELD - Physics-based particles (`wind.ts`) - **Optimized v2**

### Phase 5: 3D Visualization ✅ Complete
- [x] TUNNEL_WAVEFORM - Perspective ring tunnel (`tunnel.ts`) - **Optimized v2**

### Phase 6: Integration & Polish 🔲 In Progress
- [x] `useWebGLVisualizer.ts` - React hook for WebGL lifecycle (~220 LOC)
- [x] `WebGLVisualizer.tsx` - React component with Canvas 2D fallback (~130 LOC)
- [x] `index.ts` - Module exports for webgl folder
- [x] Replace AlbumArtVisualizer Canvas 2D with WebGL in NowPlaying.tsx
- [x] **GPU Performance Optimization Pass (v2)** - Reduced loop counts, early exits, DPR capping
- [ ] Add mode transition animations (crossfade)
- [ ] Cross-browser testing (Chrome, Firefox, Edge)
- [ ] Performance profiling and optimization

### File Structure Created
```
components/now-playing/webgl/
├── index.ts                      # Module exports
├── ShaderProgram.ts              # Shader compilation/linking (~200 LOC)
├── AudioTextureManager.ts        # Audio data → texture upload (~180 LOC)
├── SpriteAtlas.ts                # Shared texture atlas (~200 LOC)
├── WebGLVisualizerRenderer.ts    # Main renderer class (~450 LOC)
├── useWebGLVisualizer.ts         # React hook (~220 LOC)
├── WebGLVisualizer.tsx           # React component (~130 LOC)
└── shaders/
    ├── index.ts                  # Shader exports
    ├── common.ts                 # Vertex shaders + quad vertices
    ├── noise.ts                  # Simplex noise GLSL functions
    ├── sdf.ts                    # SDF shape functions
    ├── audio.ts                  # Audio utility functions
    ├── wave.ts                   # WAVE mode shader
    ├── spectrum.ts               # SPECTRUM mode shader
    ├── firefly.ts                # FIREFLY_FIELD mode shader
    ├── aurora.ts                 # AURORA_RIBBON mode shader
    ├── electric.ts               # ELECTRIC_ARC mode shader
    ├── grass.ts                  # GRASS_OSCILLOSCOPE mode shader
    ├── flame.ts                  # FLAME_SPECTRUM mode shader
    ├── stardust.ts               # STARDUST_HALO mode shader
    ├── wind.ts                   # WIND_FIELD mode shader
    └── tunnel.ts                 # TUNNEL_WAVEFORM mode shader
```

---

## WebGL Refactoring Architecture

### Unified WebGL Renderer

Create a single `WebGLVisualizerRenderer` class that manages:

```
┌──────────────────────────────────────────────────────────────┐
│                  WebGLVisualizerRenderer                      │
├──────────────────────────────────────────────────────────────┤
│ • WebGL2 context with fallback to WebGL1                     │
│ • Shared texture atlas for sprites (glow, flare, ring, etc.) │
│ • Instanced rendering for particles                          │
│ • Post-processing pipeline (bloom, trails)                   │
│ • Audio data uniforms (bass, mid, treble, time)             │
└──────────────────────────────────────────────────────────────┘
                              │
         ┌────────────────────┼────────────────────┐
         ▼                    ▼                    ▼
    ┌─────────┐         ┌─────────┐          ┌─────────┐
    │ Shader  │         │ Shader  │          │ Shader  │
    │ Program │         │ Program │          │ Program │
    │  WAVE   │         │ SPECTRUM│          │ FLAMES  │
    └─────────┘         └─────────┘          └─────────┘
```

### Core Components

#### 1. `WebGLVisualizerRenderer.ts` (New File)

```typescript
interface WebGLVisualizerRenderer {
  // Initialization
  init(canvas: HTMLCanvasElement): boolean;
  
  // Per-frame rendering
  setAudioData(frequency: Uint8Array, waveform: Uint8Array): void;
  render(mode: VisualizerMode, time: number): void;
  
  // Resource management
  resize(width: number, height: number): void;
  dispose(): void;
}
```

#### 2. Shared Resources

| Resource | Purpose | Format |
|----------|---------|--------|
| `u_audioTexture` | 256x1 frequency data | R8 texture |
| `u_waveformTexture` | 256x1 waveform data | R8 texture |
| `u_spriteAtlas` | Glow/flare/ring sprites | 512x512 RGBA |
| `u_time` | Animation time uniform | float |
| `u_resolution` | Canvas dimensions | vec2 |
| `u_bass/mid/treble` | Pre-computed energy bands | float |

#### 3. Shader Programs (GLSL ES 3.0)

Each visualization gets its own fragment shader, sharing a common vertex shader.

---

## Per-Visualization WebGL Conversion

### 1. WAVE → `wave.frag`

**Canvas 2D Issues:**
- `shadowBlur` creates expensive blur pass
- Per-vertex line drawing

**WebGL Solution:**
- Single fullscreen quad with SDF waveform
- Glow via shader math (no post-process blur needed)
- Waveform data from texture lookup

```glsl
// Pseudo-code
float waveY = texture(u_waveformTexture, vec2(uv.x, 0.0)).r;
float dist = abs(uv.y - waveY);
float glow = exp(-dist * 20.0) * 0.5; // Analytical glow
gl_FragColor = vec4(color * glow, glow);
```

**Expected Gain:** 2-3x faster

---

### 2. SPECTRUM → `spectrum.frag`

**Canvas 2D Issues:**
- 64 separate bar draws with gradients
- shadowBlur per bar
- Radial gradient for inner circle

**WebGL Solution:**
- Instanced rendering for bars (1 draw call)
- SDF-based glow (no shadow blur)
- Single pass radial gradient in shader

```glsl
// Bar SDF with soft edges
float barSDF = sdBox(localPos, vec2(barWidth, barHeight));
float glow = smoothstep(glowRadius, 0.0, barSDF);
```

**Expected Gain:** 3-5x faster

---

### 3. FLAME_SPECTRUM → `flame.frag`

**Canvas 2D Issues:**
- Dynamic particle spawning/removal (array mutations)
- `globalCompositeOperation = 'lighter'` forces readback
- Per-particle gradient fills

**WebGL Solution:**
- GPU particle system with transform feedback
- Additive blending via `gl.blendFunc(gl.ONE, gl.ONE)`
- Point sprites with flame texture

**Data Flow:**
```
Audio → Spawn buffer → Transform Feedback → Position buffer → Point Sprites
                ↑                                    │
                └──────── Feedback loop ─────────────┘
```

**Expected Gain:** 5-10x faster (300+ particles at 60fps)

---

### 4. STARDUST_HALO → `stardust.frag`

**Canvas 2D Issues:**
- 300 particles with comet trails (separate gradient per trail)
- Halo ring stroke + sparkle points
- `createRadialGradient()` per particle

**WebGL Solution:**
- Instanced lines for trails (single draw call)
- Point sprites for particle heads
- Ring rendered as SDF in fragment shader

**Trail Rendering:**
```glsl
// Line segment with distance-based fade
float trailAlpha = 1.0 - (distFromHead / trailLength);
trailAlpha *= particleLife;
```

**Expected Gain:** 5-8x faster

---

### 5. AURORA_RIBBON → `aurora.frag`

**Canvas 2D Issues:**
- 3 separate layers with different gradients
- 60 path points per layer × 3 layers = 180 lineTo calls
- Layer compositing

**WebGL Solution:**
- Noise-based ribbon shader (no geometry, pure fragment shader)
- Simplex noise for organic movement
- Single pass with layered alpha

```glsl
// Procedural ribbon
float ribbon = smoothstep(ribbonEdge, 0.0, abs(y - ribbonCenter));
ribbon *= fbm(uv * scale + time * speed); // Fractal noise modulation
```

**Expected Gain:** 3-4x faster

---

### 6. ELECTRIC_ARC → `electric.frag`

**Canvas 2D Issues:**
- Random jitter recalculated every frame per segment
- Shadow blur on arc paths
- Multiple stroke passes

**WebGL Solution:**
- Procedural lightning via noise functions
- GPU-side randomness (hash functions)
- Glow via SDF distance field

```glsl
// Procedural arc with jitter
float arc = arcNoise(uv, seed, time);
float glow = 1.0 / (1.0 + arc * arc * 100.0); // Inverse square glow
```

**Expected Gain:** 3-5x faster

---

### 7. GRASS_OSCILLOSCOPE → `grass.frag`

**Canvas 2D Issues:**
- 80 bezier curves with individual gradients
- Per-blade stroke style changes
- Stereo offset calculation

**WebGL Solution:**
- Instanced bezier rendering
- Vertex shader handles curve math
- Shared gradient as uniform

**Vertex Shader:**
```glsl
// Bezier curve evaluation
vec2 bezier(float t, vec2 p0, vec2 p1, vec2 p2, vec2 p3) {
  float u = 1.0 - t;
  return u*u*u*p0 + 3.0*u*u*t*p1 + 3.0*u*t*t*p2 + t*t*t*p3;
}
```

**Expected Gain:** 2-4x faster

---

### 8. FIREFLY_FIELD → `firefly.frag`

**Canvas 2D Issues:**
- 40 particles with radial gradients
- Per-particle `createRadialGradient()`
- Core point draw + glow draw = 2 draws per particle

**WebGL Solution:**
- Point sprites with glow texture
- Instance data: position, brightness, phase
- Single draw call for all fireflies

**Expected Gain:** 3-5x faster

---

### 9. TUNNEL_WAVEFORM → `tunnel.frag`

**Canvas 2D Issues:**
- 768 points (24 rings × 32 points)
- CPU-side perspective projection
- Per-ring color calculation

**WebGL Solution:**
- Vertex shader handles perspective math (GPU native)
- Instanced ring rendering
- Geometry shader for line width (WebGL2 workaround: wide lines via quads)

**Perspective in Vertex Shader:**
```glsl
float scale = fov / (fov + position.z);
gl_Position = vec4(position.xy * scale, 0.0, 1.0);
```

**Expected Gain:** 5-10x faster

---

### 10. WIND_FIELD → `wind.frag`

**Canvas 2D Issues:**
- 100 particles with position wrapping
- Radial gradient per particle
- Sparkle effect random checks

**WebGL Solution:**
- Transform feedback for particle physics
- Point sprites with wind texture
- Sparkle via shader noise function

**Expected Gain:** 3-5x faster

---

## Implementation Phases

### Phase 1: Core Infrastructure (Week 1-2)

**Files to Create:**
```
components/now-playing/webgl/
├── WebGLVisualizerRenderer.ts   # Main renderer class
├── ShaderProgram.ts              # Shader compilation/linking
├── AudioTextureManager.ts        # Audio data → texture upload
├── SpriteAtlas.ts                # Shared texture atlas
├── ParticleSystem.ts             # GPU particle system (transform feedback)
└── shaders/
    ├── common.vert               # Shared vertex shader
    ├── post/bloom.frag           # Post-processing bloom
    └── fullscreen.vert           # Fullscreen quad vertex shader
```

**Tasks:**
1. [ ] Create `WebGLVisualizerRenderer` with context setup and fallback
2. [ ] Implement `ShaderProgram` with caching and error handling
3. [ ] Create `AudioTextureManager` for frequency/waveform uploads
4. [ ] Build `SpriteAtlas` from existing `SpriteGenerator` outputs
5. [ ] Port `CanvasPostProcessor` bloom to WebGL

**Acceptance Criteria:**
- WebGL2 context initializes with WebGL1 fallback
- Can render a simple colored quad
- Audio data uploads to GPU at 60fps without stuttering

---

### Phase 2: Simple Visualizations (Week 2-3)

**Convert (in order of complexity):**
1. [ ] WAVE - SDF waveform with analytical glow
2. [ ] SPECTRUM - Instanced bars with SDF glow
3. [ ] FIREFLY_FIELD - Point sprites with glow texture

**Files to Create:**
```
components/now-playing/webgl/shaders/
├── wave.frag
├── spectrum.frag
└── firefly.frag
```

**Tasks:**
1. [ ] Create `wave.frag` with texture-sampled waveform
2. [ ] Create `spectrum.frag` with instanced bar rendering
3. [ ] Create `firefly.frag` with point sprite system
4. [ ] Add mode switching in `WebGLVisualizerRenderer`
5. [ ] Integration test with actual audio playback

**Acceptance Criteria:**
- All 3 modes render correctly
- 60fps on integrated GPU (Intel UHD 620)
- Visual parity with Canvas 2D versions

---

### Phase 3: Medium Complexity (Week 3-4)

**Convert:**
1. [ ] AURORA_RIBBON - Procedural noise ribbon
2. [ ] ELECTRIC_ARC - Procedural lightning
3. [ ] GRASS_OSCILLOSCOPE - Instanced bezier curves

**Files to Create:**
```
components/now-playing/webgl/shaders/
├── aurora.frag
├── electric.frag
├── grass.frag
└── utils/noise.glsl           # Shared noise functions
```

**Tasks:**
1. [ ] Implement simplex/perlin noise in `noise.glsl`
2. [ ] Create `aurora.frag` with layered noise ribbons
3. [ ] Create `electric.frag` with procedural arcs
4. [ ] Create `grass.frag` with instanced curves
5. [ ] Profile and optimize shader math

**Acceptance Criteria:**
- All 3 modes at 60fps
- Noise-based effects look organic (not mechanical)
- Memory usage stable (no leaks)

---

### Phase 4: Particle Systems (Week 4-5)

**Convert:**
1. [ ] FLAME_SPECTRUM - GPU particle system with spawn/death
2. [ ] STARDUST_HALO - Particle trails + ring
3. [ ] WIND_FIELD - Physics-based particles

**Files to Create:**
```
components/now-playing/webgl/
├── TransformFeedbackParticles.ts  # GPU particle system
└── shaders/
    ├── particles/update.vert      # Particle physics
    ├── particles/render.vert      # Particle rendering
    ├── flame.frag
    ├── stardust.frag
    └── wind.frag
```

**Tasks:**
1. [ ] Implement transform feedback particle system
2. [ ] Create spawn/death logic in update shader
3. [ ] Port flame particle colors and behavior
4. [ ] Port stardust trail rendering
5. [ ] Port wind physics and sparkle effects

**Acceptance Criteria:**
- 500+ particles at 60fps
- Particle trails smooth (no flickering)
- Memory stable with spawn/death cycle

---

### Phase 5: 3D Visualization (Week 5-6)

**Convert:**
1. [ ] TUNNEL_WAVEFORM - 3D ring tunnel with perspective

**Files to Create:**
```
components/now-playing/webgl/shaders/
├── tunnel.vert                # Perspective projection
└── tunnel.frag                # Ring coloring
```

**Tasks:**
1. [ ] Create ring geometry buffer
2. [ ] Implement perspective projection in vertex shader
3. [ ] Port ring coloring and audio distortion
4. [ ] Add depth-based alpha fade
5. [ ] Optimize for high ring count

**Acceptance Criteria:**
- 24 rings × 32 points at 60fps
- Smooth perspective depth
- Audio-reactive distortion visible

---

### Phase 6: Integration & Polish (Week 6-7)

**Tasks:**
1. [ ] Replace `AlbumArtVisualizer` Canvas 2D with WebGL renderer
2. [ ] Add graceful fallback to Canvas 2D if WebGL unavailable
3. [ ] Implement mode transition animations (crossfade)
4. [ ] Memory profiling and leak detection
5. [ ] Cross-browser testing (Chrome, Firefox, Edge)
6. [ ] Device testing (integrated GPU, discrete GPU, MacOS Metal)

**Files to Modify:**
```
components/now-playing/AlbumArtVisualizer.tsx  # Replace Canvas 2D
components/NowPlaying.tsx                      # Mode switching
```

**Acceptance Criteria:**
- Zero WebGL errors in console
- Fallback works on old browsers
- No memory leaks after 1 hour playback
- Consistent 60fps across all tested devices

---

## Technical Specifications

### WebGL Requirements

| Feature | Requirement | Fallback |
|---------|------------|----------|
| WebGL Version | WebGL2 preferred | WebGL1 with extensions |
| Extensions | `OES_texture_float` | Normalized textures |
| Max Texture Size | 2048x2048 | Scale down |
| Transform Feedback | Required for particles | CPU particle fallback |

### Shader Precision

```glsl
// Use mediump where possible for mobile perf
precision mediump float;
precision mediump int;

// highp only for positions and time
highp float u_time;
highp vec2 u_resolution;
```

### Memory Budget

| Resource | Max Size | Notes |
|----------|----------|-------|
| Sprite Atlas | 512x512 RGBA (1MB) | Shared across all modes |
| Audio Texture | 256x1 R8 (256B) | Updated every frame |
| Particle Buffer | 2000 particles (48KB) | Max for transform feedback |
| Ring Buffer | 1024 vertices (12KB) | For tunnel mode |

---

## Risk Assessment

| Risk | Impact | Mitigation |
|------|--------|------------|
| WebGL not available | High | Canvas 2D fallback path |
| Transform feedback unsupported | Medium | CPU particle fallback for those modes |
| Shader compilation fails | Medium | Precompile and cache, error logging |
| Performance regression | High | A/B benchmark before replacing |
| Visual parity loss | Medium | Side-by-side comparison tool |

---

## Success Metrics

### Performance Targets

| Metric | Current | Target |
|--------|---------|--------|
| Min FPS (TUNNEL) | 20 | 60 |
| Min FPS (STARDUST) | 20 | 60 |
| Avg FPS (all modes) | 35 | 60+ |
| GPU Memory | N/A | <50MB |
| Frame Time | 28ms avg | <16ms |

### Quality Targets

- Visual parity score: >90% (automated pixel comparison)
- Audio sync latency: <1 frame (16ms)
- Zero WebGL context lost events in 1 hour test

---

## File Structure (Final)

```
components/now-playing/
├── AlbumArtVisualizer.tsx          # Updated to use WebGL
├── CanvasPostProcessor.ts          # Keep for fallback
├── SpriteGenerator.ts              # Keep for sprite atlas source
└── webgl/
    ├── WebGLVisualizerRenderer.ts
    ├── ShaderProgram.ts
    ├── AudioTextureManager.ts
    ├── SpriteAtlas.ts
    ├── ParticleSystem.ts
    ├── TransformFeedbackParticles.ts
    └── shaders/
        ├── common.vert
        ├── fullscreen.vert
        ├── wave.frag
        ├── spectrum.frag
        ├── flame.frag
        ├── stardust.frag
        ├── aurora.frag
        ├── electric.frag
        ├── grass.frag
        ├── firefly.frag
        ├── tunnel.vert
        ├── tunnel.frag
        ├── wind.frag
        ├── post/
        │   └── bloom.frag
        ├── particles/
        │   ├── update.vert
        │   └── render.vert
        └── utils/
            ├── noise.glsl
            ├── sdf.glsl
            └── audio.glsl
```

---

## Dependencies

### NPM Packages (Optional)

| Package | Purpose | Size |
|---------|---------|------|
| `twgl.js` | WebGL boilerplate | ~15KB |
| `gl-matrix` | Matrix math | ~30KB |

**Recommendation:** Keep dependencies minimal. Implement custom helpers for simple operations.

---

## Timeline Summary

| Phase | Duration | Deliverables |
|-------|----------|--------------|
| 1. Infrastructure | 2 weeks | Core renderer, audio texture, sprite atlas |
| 2. Simple Modes | 1 week | WAVE, SPECTRUM, FIREFLY |
| 3. Medium Modes | 1 week | AURORA, ELECTRIC, GRASS |
| 4. Particle Modes | 1 week | FLAME, STARDUST, WIND |
| 5. 3D Mode | 1 week | TUNNEL |
| 6. Integration | 1 week | Polish, testing, fallback |
| **Total** | **7 weeks** | All 10 modes in WebGL |

---

## Appendix: Shader Examples

### A. SDF Glow (wave.frag)

```glsl
#version 300 es
precision mediump float;

uniform sampler2D u_waveformTexture;
uniform vec2 u_resolution;
uniform float u_time;
uniform vec3 u_color;

out vec4 fragColor;

void main() {
    vec2 uv = gl_FragCoord.xy / u_resolution;
    
    // Sample waveform
    float waveY = texture(u_waveformTexture, vec2(uv.x, 0.0)).r;
    waveY = waveY * 0.5 + 0.25; // Remap to [0.25, 0.75]
    
    // Distance to waveform
    float dist = abs(uv.y - waveY);
    
    // Analytical glow
    float glow = exp(-dist * 30.0);
    float core = exp(-dist * 100.0);
    
    vec3 color = u_color * glow + vec3(1.0) * core;
    fragColor = vec4(color, glow);
}
```

### B. Instanced Bars (spectrum.vert)

```glsl
#version 300 es
precision mediump float;

// Per-vertex
in vec2 a_position;

// Per-instance
in float a_frequency;
in float a_instanceId;

uniform vec2 u_resolution;
uniform float u_barWidth;
uniform float u_gap;

out float v_frequency;
out float v_barIndex;

void main() {
    float x = a_instanceId * (u_barWidth + u_gap) + a_position.x * u_barWidth;
    float y = a_position.y * a_frequency;
    
    vec2 pos = vec2(x, y) / u_resolution * 2.0 - 1.0;
    gl_Position = vec4(pos, 0.0, 1.0);
    
    v_frequency = a_frequency;
    v_barIndex = a_instanceId;
}
```

### C. Simplex Noise (noise.glsl)

```glsl
// 2D Simplex noise
vec3 permute(vec3 x) { return mod(((x*34.0)+1.0)*x, 289.0); }

float snoise(vec2 v) {
    const vec4 C = vec4(0.211324865405187, 0.366025403784439,
                        -0.577350269189626, 0.024390243902439);
    vec2 i  = floor(v + dot(v, C.yy));
    vec2 x0 = v - i + dot(i, C.xx);
    vec2 i1 = (x0.x > x0.y) ? vec2(1.0, 0.0) : vec2(0.0, 1.0);
    vec4 x12 = x0.xyxy + C.xxzz;
    x12.xy -= i1;
    i = mod(i, 289.0);
    vec3 p = permute(permute(i.y + vec3(0.0, i1.y, 1.0)) + i.x + vec3(0.0, i1.x, 1.0));
    vec3 m = max(0.5 - vec3(dot(x0,x0), dot(x12.xy,x12.xy), dot(x12.zw,x12.zw)), 0.0);
    m = m*m; m = m*m;
    vec3 x = 2.0 * fract(p * C.www) - 1.0;
    vec3 h = abs(x) - 0.5;
    vec3 ox = floor(x + 0.5);
    vec3 a0 = x - ox;
    m *= 1.79284291400159 - 0.85373472095314 * (a0*a0 + h*h);
    vec3 g;
    g.x = a0.x * x0.x + h.x * x0.y;
    g.yz = a0.yz * x12.xz + h.yz * x12.yw;
    return 130.0 * dot(m, g);
}
```

---

## Document Metadata

- **Created:** 2025-01-15
- **Author:** ViiB Development Team
- **Version:** 1.8
- **Status:** In Progress
- **Last Updated:** 2025-01-27

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2025-01-15 | Initial planning document |
| 1.1 | 2025-01-25 | Updated with accurate LOC counts, added progress tracking, fixed visualizer mode list |
| 1.2 | 2025-01-27 | Completed Phases 1-5: All core infrastructure and shaders implemented |
| 1.3 | 2025-01-27 | Phase 6 integration: WebGLVisualizer now used in NowPlaying.tsx with Canvas 2D fallback |
| 1.4 | 2025-01-27 | Bug fixes: Fixed GLSL function conflicts (hash, fbm) causing shader compilation failures |
| 1.5 | 2025-01-27 | Bug fix: Album art overlay now works - simplified opacity handling |
| 1.6 | 2025-01-27 | Bug fix: Album art opacity settings now respected - removed canvas inline opacity override |
| 1.7 | 2025-01-27 | Bug fix: Differentiated opacity behavior by visualizer type. **Legacy (WebGL/Canvas)**: Album art dims by setting, visualizer at full opacity. **Milkdrop**: Album art at full opacity, visualizer dims by setting. |
| 1.8 | 2025-01-27 | **GPU Performance Optimization Pass (v2)**: Optimized 5 heavy shaders (STARDUST, WIND, FLAME, TUNNEL, ELECTRIC) with reduced loop counts, early exits, simplified math, and DPR capping. Expected 1.3-2x performance improvement. |