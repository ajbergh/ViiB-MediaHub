/**
 * DJ WebGL Module
 * 
 * High-performance WebGL2 rendering for DJ waveforms and visualizations.
 * Exports GPU renderers and hooks; animation targets depend on host frame scheduling.
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
export { DJWebGLWaveformDeck } from './DJWebGLWaveformDeck';

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
