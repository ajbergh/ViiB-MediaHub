/**
 * WebGL Visualizer Module Exports
 * 
 * Entry point for the WebGL visualization system.
 */

// React component and hook
export { WebGLVisualizer } from './WebGLVisualizer';
export { useWebGLVisualizer } from './useWebGLVisualizer';

// Core classes
export { WebGLVisualizerRenderer } from './WebGLVisualizerRenderer';
export type { WebGLVisualizerOptions } from './WebGLVisualizerRenderer';

export { ShaderProgram, ShaderCache } from './ShaderProgram';
export type { ShaderSource } from './ShaderProgram';

export { AudioTextureManager } from './AudioTextureManager';
export type { AudioEnergy } from './AudioTextureManager';

export { SpriteAtlas } from './SpriteAtlas';
export type { SpriteUV, SpriteType } from './SpriteAtlas';

// Shader sources (for custom implementations)
export * from './shaders/common';
export { noiseGLSL } from './shaders/noise';
export { sdfGLSL } from './shaders/sdf';
export { audioGLSL } from './shaders/audio';

// Mode-specific shaders
export { waveFragmentShader } from './shaders/wave';
export { spectrumFragmentShader } from './shaders/spectrum';
export { fireflyFragmentShader } from './shaders/firefly';
export { auroraFragmentShader } from './shaders/aurora';
export { electricFragmentShader } from './shaders/electric';
export { grassFragmentShader } from './shaders/grass';
export { flameFragmentShader } from './shaders/flame';
export { stardustFragmentShader } from './shaders/stardust';
export { windFragmentShader } from './shaders/wind';
export { tunnelFragmentShader } from './shaders/tunnel';
