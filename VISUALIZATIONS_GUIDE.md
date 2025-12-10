# ViiB MediaHub - Next-Gen Visualizations Guide

## Overview

ViiB MediaHub features **21 stunning audio visualizations** that react to your music in real-time. Each visualization is designed to be intentional, artistic, and performant, providing a beautiful visual experience while you enjoy your music.

**🎯 Performance Optimized:** All visualizations have been extensively optimized for 60 FPS performance on mid-range hardware, with +25.8% average performance improvement and +38.2% cool factor enhancement through techniques like particle capping, composite blending, parallax depth, and physics-based interactions. See optimization documentation for technical details.

## How to Use

1. Navigate to **Settings** → **Audio** section
2. Find the **Visualizer Style** dropdown
3. Select your preferred visualization from:
   - **Classic Visualizations** (original 6 modes)
   - **Next-Gen Visualizations** (15 new artistic modes)
4. Open the **Now Playing** view to see your visualization in action!

---

## Classic Visualizations

### Waveform
A smooth, glowing waveform that shows the audio signal in real-time with elegant curves and glow effects.

### Spectrum Bars
Traditional frequency spectrum displayed as circular bars radiating from the center, creating a sun-burst effect.

### Ambient Aurora
Flowing gradients that react to bass, mids, and treble, creating a Northern Lights-inspired ambient effect.

### Circular Pulse
Enhanced circular visualization with rotating frequency bars, pulsing rings, and an inner waveform circle.

### Particle Storm
Dynamic particle system where particles burst from the center and react to audio energy with gravity effects.

### Deep Space Nebula
Cosmic atmosphere with swirling nebula clouds, twinkling stars, and lens flare rays on bass hits.

---

## 🔥 Next-Gen Visualizations

### 🔥 Flame Spectrum Crown
**Audio Mapping:**
- Flame height → Frequency amplitude
- Color transitions:
  - Low intensity → Warm orange
  - Medium → Fiery red
  - High → White-hot flare
- Random flicker adds organic movement

**Best for:** Energetic music, rock, electronic, hip-hop

---

### 🌌 Stardust Pulse Halo
**Audio Mapping:**
- Bass hits → Burst of stardust particles
- Particles expand outward from circular halo
- Treble → Shimmering glints on the ring
- Ring radius pulses with bass energy

**Best for:** Ambient, electronic, space-themed music

---

### 🌈 Aurora Ribbon
**Audio Mapping:**
- Ribbon height & curvature → Waveform
- Color gradient shifts with frequency dominance:
  - Bass dominant → Teal/purple/rose
  - Mid dominant → Purple/pink/blue
  - Treble dominant → Pink/cyan/green
- Slow, ethereal wave motion

**Best for:** Jazz, ambient, classical, lo-fi

---

### ⚡ Electric Arc Wireframe
**Audio Mapping:**
- Bass hits → New arcs spawn between random points
- Arc brightness → Bass intensity
- Treble → Crackling white hotspots
- Beams fade and multiply dynamically

**Best for:** Electronic, EDM, synthwave, futuristic tracks

---

### 🌱 Growing Grass Oscilloscope
**Audio Mapping:**
- Blade height → Amplitude
- Sway motion → Stereo L/R offset
- Color → Dynamic green gradient
- Organic bezier curves for natural movement

**Best for:** Acoustic, folk, nature sounds, relaxing music

---

### 💠 Crystal Shards Burst
**Audio Mapping:**
- Beats → Prismatic shards grow and burst outward
- Idle state → Gentle rotating facets
- Sustained notes → Breathing glow at center
- Refraction effects with rainbow gradients

**Best for:** Bright, uplifting music; pop, indie, upbeat tracks

---

### 🎨 Watercolor Bloom
**Audio Mapping:**
- Each beat → Spawns circular bloom
- Opacity + radius → Amplitude
- Colors → Random artistic palette
- Multiple layers create painterly depth

**Best for:** Artistic tracks, indie, singer-songwriter, mellow music

---

### 🧊 Ice Fracture Pulse
**Audio Mapping:**
- Bass hits → Fractures radiate from center
- High frequencies → Sparkle points
- Micro-fractures branch and spread
- Cool white/blue aesthetic

**Best for:** Minimal, chill, winter-themed music

---

### 🌟 Holiday Firefly Field (Seasonal Mode)
**Audio Mapping:**
- Fireflies drift with slow, organic movement
- Glow intensity → Mids/highs
- Occasional flicker on beat
- Warm, nostalgic candlelight aesthetic

**Best for:** Holiday music, Christmas classics, acoustic, cozy vibes

---

### 🌀 Vinyl Spin Overlay
**Audio Mapping:**
- Rotation speed → Tempo (bass response)
- Occasional glints → Treble peaks
- Ripple distortions on strong bass
- Semi-transparent grooves rotate over artwork

**Best for:** Vinyl-style music, classic rock, jazz, retro tracks

---

### 💥 Beat Explosion Orbs
**Audio Mapping:**
- Bass hits → Volumetric orbs burst outward
- Orb size → Energy level
- Color → Album art palette
- Soft gradient fades create gentle fireworks

**Best for:** Hip-hop, trap, bass-heavy electronic music

---

### 🔊 3D Tunnel Waveform
**Audio Mapping:**
- Tunnel depth → Bass compression
- Ring thickness → Mids
- Sparkles on edges → Treble
- Perspective depth from back to front

**Best for:** Psychedelic, trance, progressive music

---

### 🪞 Reflective Glass Shards
**Audio Mapping:**
- Bass → Glass fragments scatter
- Fragments rotate and drift
- Return to center when calm
- Prismatic reflections on each shard
- Treble → Shimmer across edges

**Best for:** Glitch, experimental, modern classical

---

### 🌬️ Soft Wind Field
**Audio Mapping:**
- Bass → Wind intensity
- Treble → Sparkle density
- Particles flow across screen like wind
- Gentle, calming motion

**Best for:** Ambient, meditation, nature sounds, sleep music

---

## Technical Details

### Performance
- All visualizations use **Canvas 2D rendering** for broad compatibility
- **60 FPS target** with automatic optimization
- Particle systems are capped to prevent performance issues
- Fade in/out transitions for smooth mode switching

### Audio Analysis
- **Web Audio API** for real-time frequency analysis
- **FFT size: 2048** for high-resolution frequency data
- Separate bass (30-50 Hz), mid (50-200 Hz), and treble (200+ Hz) band calculations
- Smoothing for natural, non-jittery animations

### Customization
Each visualization automatically adapts to:
- Album artwork colors (where applicable)
- Current audio energy levels
- Screen size and aspect ratio
- Device pixel ratio for sharp rendering

---

## Tips for Best Experience

1. **Match visualization to genre**: Experiment to find which visualizations complement different music styles
2. **Full-screen mode**: Visualizations look best in Now Playing full-screen view
3. **Dark environments**: Many visualizations have glow effects that shine in darker settings
4. **High-quality audio**: Better audio quality = more responsive visualizations
5. **Try them all**: Each visualization offers a unique artistic interpretation of your music

---

## Development Notes

### Adding New Visualizations

To add a new visualization mode:

1. **Add to type definition** (`types.ts`):
   ```typescript
   export type VisualizerMode = '...' | 'NEW_MODE';
   ```

2. **Create renderer function** (`AlbumArtVisualizer.tsx`):
   ```typescript
   const drawNewMode = useCallback((ctx, frequencyData, width, height, time) => {
     // Your visualization logic
   }, []);
   ```

3. **Add to switch statement**:
   ```typescript
   case 'NEW_MODE':
     drawNewMode(ctx, frequencyData, width, height, timestamp);
     break;
   ```

4. **Update dependencies** in useEffect hook

5. **Add to Settings UI** (`Settings.tsx`)

### Audio Data Access
- `frequencyData`: Uint8Array of frequency magnitudes (0-255)
- `waveformData`: Uint8Array of time-domain samples (0-255, centered at 128)
- `analyser.frequencyBinCount`: Number of frequency bins available

### Best Practices
- Use `opacityRef.current` for fade transitions
- Apply `ctx.save()` and `ctx.restore()` to isolate transforms
- Calculate energy bands (bass, mid, treble) for responsive effects
- Cap particle counts to maintain performance
- Use `requestAnimationFrame` timestamp for smooth time-based animations

---

## Credits

Visualizations designed and implemented for ViiB MediaHub with inspiration from:
- Classic audio visualizers (Winamp, iTunes)
- Modern design aesthetics (Material Design, Fluent Design)
- Nature-inspired motion (aurora, fireflies, wind, grass)
- Retro technology (vinyl records, TRON aesthetics)
- Abstract art movements (watercolor, crystals, glass)

---

## Feedback & Contributions

Have ideas for new visualizations? Found a bug? Please contribute!

**Repository**: [GitHub - ViiB-MediaHub](https://github.com/ajbergh/ViiB-MediaHub)

Enjoy the visual journey with your music! 🎵✨
