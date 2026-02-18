/**
 * DJ WebGL Module
 * 
 * High-performance WebGL2 rendering for DJ waveforms and visualizations.
 * Provides GPU-accelerated rendering for smooth 60+ FPS performance.
 * 
 * @module components/dj/v2/webgl
 */

// Core renderer
export { DJWebGLRenderer } from './DJWebGLRenderer';
export type { DJWaveformRenderState, DJWebGLRendererOptions, HotCue } from './DJWebGLRenderer';

// React hook
export { useDJWebGL, useDJWebGLAnimation } from './useDJWebGL';
export type { UseDJWebGLOptions, UseDJWebGLReturn } from './useDJWebGL';

// React component
export { DJWebGLWaveform } from './DJWebGLWaveform';

// Shaders (for advanced customization)
export {
  djWaveformVertexShader,
  djWaveformFragmentShader,
  djBeatGridFragmentShader,
  djPlayheadFragmentShader,
  djHotCueFragmentShader,
  djOverviewFragmentShader,
  djCuePointFragmentShader,
  FULLSCREEN_QUAD_VERTICES,
} from './DJWaveformShaders';
