/**
 * DJ WebGL Renderer
 * 
 * High-performance WebGL2 rendering engine for DJ waveforms.
 * Manages WebGL context, shader programs, textures, and per-deck rendering.
 * 
 * Architecture:
 * - WebGL2 primary with WebGL1 fallback
 * - Waveform peak data uploaded as Float32 texture
 * - Multi-layer compositing (waveform → beat grid → markers → playhead)
 * - Shared context across all DJ canvases
 * 
 * @module DJWebGLRenderer
 */

import {
  FULLSCREEN_QUAD_VERTICES,
  djWaveformVertexShader,
  djWaveformVertexShaderWebGL1,
  djWaveformFragmentShader,
  djBeatGridFragmentShader,
  djPlayheadFragmentShader,
  djHotCueFragmentShader,
  djOverviewFragmentShader,
  djCuePointFragmentShader,
  toDJWebGL1FragmentShader,
} from './DJWaveformShaders';
import { getPreferredWebGLVersion } from '../../../../lib/webglSafety';

export type DeckId = 'A' | 'B';

export interface HotCue {
  position: number;  // Position in seconds
  color: string;     // CSS color
  label?: string;
}

export interface DJWaveformRenderState {
  /** Waveform peak data (0-1 normalized amplitudes) */
  peaks: number[] | Float32Array | null;
  /** Current playback position in seconds */
  position: number;
  /** Total track duration in seconds */
  duration: number;
  /** BPM for beat grid */
  bpm: number;
  /** Beat grid positions in seconds */
  beatGrid: number[] | null;
  /** Cue point position in seconds */
  cuePoint: number;
  /** Hot cue markers */
  hotCues: HotCue[];
  /** Visible time window in seconds */
  visibleSeconds: number;
  /** Deck identifier */
  deck: DeckId;
}

export interface DJWebGLRendererOptions {
  /** Deck A accent color */
  deckAColor?: [number, number, number];
  /** Deck B accent color */
  deckBColor?: [number, number, number];
  /** Playhead color */
  playheadColor?: [number, number, number];
  /** Cue point color */
  cueColor?: [number, number, number];
}

const DEFAULT_OPTIONS: Required<DJWebGLRendererOptions> = {
  deckAColor: [0.231, 0.510, 0.965],     // #3b82f6 (blue)
  deckBColor: [0.545, 0.361, 0.965],     // #8b5cf6 (purple)
  playheadColor: [1.0, 0.2, 0.2],        // #ff3333 (red)
  cueColor: [0.961, 0.620, 0.043],       // #f59e0b (amber)
};

interface ShaderProgram {
  program: WebGLProgram;
  uniformLocations: Map<string, WebGLUniformLocation | null>;
}

export class DJWebGLRenderer {
  private canvas: HTMLCanvasElement | null = null;
  private gl: WebGL2RenderingContext | WebGLRenderingContext | null = null;
  private isWebGL2: boolean = false;
  
  // Geometry
  private quadVAO: WebGLVertexArrayObject | null = null;
  private quadBuffer: WebGLBuffer | null = null;
  
  // Shader programs
  private waveformProgram: ShaderProgram | null = null;
  private beatGridProgram: ShaderProgram | null = null;
  private playheadProgram: ShaderProgram | null = null;
  private hotCueProgram: ShaderProgram | null = null;
  private overviewProgram: ShaderProgram | null = null;
  private cuePointProgram: ShaderProgram | null = null;
  
  // Textures (per-deck waveform data)
  private waveformTextureA: WebGLTexture | null = null;
  private waveformTextureB: WebGLTexture | null = null;
  private textureWidth: number = 4096; // High resolution for smooth scrolling
  
  // State
  private width: number = 0;
  private height: number = 0;
  private options: Required<DJWebGLRendererOptions>;
  private initialized: boolean = false;
  private contextLost: boolean = false;
  
  // Context loss handlers
  private contextLostHandler: ((e: Event) => void) | null = null;
  private contextRestoredHandler: ((e: Event) => void) | null = null;
  
  // Pre-allocated arrays
  private peakDataA: Float32Array = new Float32Array(this.textureWidth);
  private peakDataB: Float32Array = new Float32Array(this.textureWidth);

  constructor(options: DJWebGLRendererOptions = {}) {
    this.options = { ...DEFAULT_OPTIONS, ...options };
  }

  /**
   * Initializes WebGL context and resources
   */
  init(canvas: HTMLCanvasElement): boolean {
    if (this.initialized) {
      console.warn('[DJWebGL] Already initialized');
      return true;
    }
    
    this.canvas = canvas;
    
    const preferWebGL1 = getPreferredWebGLVersion() === 'webgl1';
    // Modern WKWebView supports WebGL 2. Older system WebKit installations
    // fall through to the WebGL 1 shader path.
    let gl: WebGL2RenderingContext | WebGLRenderingContext | null = preferWebGL1 ? null : canvas.getContext('webgl2', {
      alpha: true,
      antialias: false,
      depth: false,
      stencil: false,
      premultipliedAlpha: true,
      preserveDrawingBuffer: false,
    });
    
    if (gl) {
      this.isWebGL2 = true;
      console.log('[DJWebGL] Using WebGL2');
    } else {
      gl = canvas.getContext('webgl', {
        alpha: true,
        antialias: false,
        depth: false,
        stencil: false,
        premultipliedAlpha: true,
      });
      
      if (gl) {
        this.isWebGL2 = false;
        console.log('[DJWebGL] Using WebGL1 fallback');
      }
    }
    
    if (!gl) {
      console.error('[DJWebGL] WebGL not supported');
      return false;
    }
    
    this.gl = gl;
    
    try {
      // Create geometry
      this.createQuadGeometry();
      
      // Compile shader programs
      this.waveformProgram = this.createProgram(
        this.isWebGL2 ? djWaveformVertexShader : djWaveformVertexShaderWebGL1,
        djWaveformFragmentShader
      );
      
      this.beatGridProgram = this.createProgram(
        this.isWebGL2 ? djWaveformVertexShader : djWaveformVertexShaderWebGL1,
        djBeatGridFragmentShader
      );
      
      this.playheadProgram = this.createProgram(
        this.isWebGL2 ? djWaveformVertexShader : djWaveformVertexShaderWebGL1,
        djPlayheadFragmentShader
      );
      
      this.hotCueProgram = this.createProgram(
        this.isWebGL2 ? djWaveformVertexShader : djWaveformVertexShaderWebGL1,
        djHotCueFragmentShader
      );
      
      this.overviewProgram = this.createProgram(
        this.isWebGL2 ? djWaveformVertexShader : djWaveformVertexShaderWebGL1,
        djOverviewFragmentShader
      );
      
      this.cuePointProgram = this.createProgram(
        this.isWebGL2 ? djWaveformVertexShader : djWaveformVertexShaderWebGL1,
        djCuePointFragmentShader
      );
      
      // Create waveform textures
      this.createWaveformTextures();
      
      // Configure GL state
      gl.enable(gl.BLEND);
      gl.blendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA);
      gl.disable(gl.DEPTH_TEST);
      gl.disable(gl.CULL_FACE);
      
      // Set initial viewport
      this.resize(canvas.clientWidth, canvas.clientHeight);
      
      // Set up context loss handling
      this.setupContextLossHandling(canvas);
      
      this.initialized = true;
      console.log('[DJWebGL] Initialization complete');
      return true;
      
    } catch (error) {
      console.error('[DJWebGL] Initialization failed:', error);
      this.dispose();
      return false;
    }
  }

  /**
   * Creates the fullscreen quad geometry
   */
  private createQuadGeometry(): void {
    const gl = this.gl!;
    
    this.quadBuffer = gl.createBuffer();
    gl.bindBuffer(gl.ARRAY_BUFFER, this.quadBuffer);
    gl.bufferData(gl.ARRAY_BUFFER, FULLSCREEN_QUAD_VERTICES, gl.STATIC_DRAW);
    
    if (this.isWebGL2) {
      const gl2 = gl as WebGL2RenderingContext;
      this.quadVAO = gl2.createVertexArray();
      gl2.bindVertexArray(this.quadVAO);
      
      gl.enableVertexAttribArray(0);
      gl.vertexAttribPointer(0, 2, gl.FLOAT, false, 0, 0);
      
      gl2.bindVertexArray(null);
    }
    
    gl.bindBuffer(gl.ARRAY_BUFFER, null);
  }

  /**
   * Sets up WebGL context loss and restoration handling
   */
  private setupContextLossHandling(canvas: HTMLCanvasElement): void {
    this.contextLostHandler = (e: Event) => {
      e.preventDefault();
      this.contextLost = true;
      console.warn('[DJWebGL] Context lost - will attempt to restore');
    };
    
    this.contextRestoredHandler = () => {
      console.log('[DJWebGL] Context restored - reinitializing');
      this.contextLost = false;
      
      // Reinitialize all GPU resources
      try {
        this.createQuadGeometry();
        this.waveformProgram = this.createProgram(
          this.isWebGL2 ? djWaveformVertexShader : djWaveformVertexShaderWebGL1,
          djWaveformFragmentShader
        );
        this.beatGridProgram = this.createProgram(
          this.isWebGL2 ? djWaveformVertexShader : djWaveformVertexShaderWebGL1,
          djBeatGridFragmentShader
        );
        this.playheadProgram = this.createProgram(
          this.isWebGL2 ? djWaveformVertexShader : djWaveformVertexShaderWebGL1,
          djPlayheadFragmentShader
        );
        this.hotCueProgram = this.createProgram(
          this.isWebGL2 ? djWaveformVertexShader : djWaveformVertexShaderWebGL1,
          djHotCueFragmentShader
        );
        this.overviewProgram = this.createProgram(
          this.isWebGL2 ? djWaveformVertexShader : djWaveformVertexShaderWebGL1,
          djOverviewFragmentShader
        );
        this.cuePointProgram = this.createProgram(
          this.isWebGL2 ? djWaveformVertexShader : djWaveformVertexShaderWebGL1,
          djCuePointFragmentShader
        );
        this.createWaveformTextures();
        
        // Re-upload waveform data
        this.updateWaveformData('A', this.peakDataA);
        this.updateWaveformData('B', this.peakDataB);
        
        console.log('[DJWebGL] Context restored successfully');
      } catch (error) {
        console.error('[DJWebGL] Failed to restore context:', error);
      }
    };
    
    canvas.addEventListener('webglcontextlost', this.contextLostHandler);
    canvas.addEventListener('webglcontextrestored', this.contextRestoredHandler);
  }

  /**
   * Creates waveform data textures
   */
  private createWaveformTextures(): void {
    const gl = this.gl!;
    
    const createTexture = (): WebGLTexture | null => {
      const texture = gl.createTexture();
      gl.bindTexture(gl.TEXTURE_2D, texture);
      
      // Use RED format for single-channel float data
      if (this.isWebGL2) {
        const gl2 = gl as WebGL2RenderingContext;
        gl2.texImage2D(
          gl2.TEXTURE_2D, 0, gl2.R32F,
          this.textureWidth, 1, 0,
          gl2.RED, gl2.FLOAT, null
        );
      } else {
        // WebGL1: Use LUMINANCE as fallback
        gl.texImage2D(
          gl.TEXTURE_2D, 0, gl.LUMINANCE,
          this.textureWidth, 1, 0,
          gl.LUMINANCE, gl.UNSIGNED_BYTE, null
        );
      }
      
      // R32F is not linearly filterable without this optional capability, even
      // in WebGL 2. An incomplete sampler silently produces a flat waveform.
      // Keep older WebViews valid with nearest filtering; WebGL 1 uses bytes.
      const filter = this.isWebGL2 && !gl.getExtension('OES_texture_float_linear')
        ? gl.NEAREST : gl.LINEAR;
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, filter);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, filter);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE);
      
      return texture;
    };
    
    this.waveformTextureA = createTexture();
    this.waveformTextureB = createTexture();
    
    gl.bindTexture(gl.TEXTURE_2D, null);
  }

  /**
   * Compiles and links a shader program
   */
  private createProgram(vertexSource: string, fragmentSource: string): ShaderProgram {
    const gl = this.gl!;
    
    // Compile vertex shader
    const vertexShader = gl.createShader(gl.VERTEX_SHADER)!;
    gl.shaderSource(vertexShader, vertexSource);
    gl.compileShader(vertexShader);
    
    if (!gl.getShaderParameter(vertexShader, gl.COMPILE_STATUS)) {
      const info = gl.getShaderInfoLog(vertexShader);
      throw new Error(`Vertex shader compilation failed:\n${info}`);
    }
    
    // Compile fragment shader
    const fragmentShader = gl.createShader(gl.FRAGMENT_SHADER)!;
    gl.shaderSource(fragmentShader, this.isWebGL2 ? fragmentSource : toDJWebGL1FragmentShader(fragmentSource));
    gl.compileShader(fragmentShader);
    
    if (!gl.getShaderParameter(fragmentShader, gl.COMPILE_STATUS)) {
      const info = gl.getShaderInfoLog(fragmentShader);
      throw new Error(`Fragment shader compilation failed:\n${info}`);
    }
    
    // Link program
    const program = gl.createProgram()!;
    gl.attachShader(program, vertexShader);
    gl.attachShader(program, fragmentShader);
    gl.linkProgram(program);
    
    if (!gl.getProgramParameter(program, gl.LINK_STATUS)) {
      const info = gl.getProgramInfoLog(program);
      throw new Error(`Shader program link failed:\n${info}`);
    }
    
    // Clean up shaders
    gl.deleteShader(vertexShader);
    gl.deleteShader(fragmentShader);
    
    return {
      program,
      uniformLocations: new Map(),
    };
  }

  /**
   * Gets (or caches) a uniform location
   */
  private getUniform(prog: ShaderProgram, name: string): WebGLUniformLocation | null {
    if (!prog.uniformLocations.has(name)) {
      prog.uniformLocations.set(name, this.gl!.getUniformLocation(prog.program, name));
    }
    return prog.uniformLocations.get(name) || null;
  }

  /**
   * Uploads waveform peak data to GPU texture
   */
  updateWaveformData(deck: DeckId, peaks: number[] | Float32Array | null): void {
    if (!this.gl || !peaks || peaks.length === 0) return;
    
    const gl = this.gl;
    const texture = deck === 'A' ? this.waveformTextureA : this.waveformTextureB;
    const peakData = deck === 'A' ? this.peakDataA : this.peakDataB;
    
    if (!texture) return;
    
    // Resample peaks to texture width
    const inputLength = peaks.length;
    const step = inputLength / this.textureWidth;
    
    for (let i = 0; i < this.textureWidth; i++) {
      const srcIndex = Math.floor(i * step);
      peakData[i] = peaks[srcIndex] || 0;
    }
    
    gl.bindTexture(gl.TEXTURE_2D, texture);
    
    if (this.isWebGL2) {
      const gl2 = gl as WebGL2RenderingContext;
      gl2.texSubImage2D(
        gl2.TEXTURE_2D, 0, 0, 0,
        this.textureWidth, 1,
        gl2.RED, gl2.FLOAT, peakData
      );
    } else {
      // WebGL1: Convert to Uint8
      const uint8Data = new Uint8Array(this.textureWidth);
      for (let i = 0; i < this.textureWidth; i++) {
        uint8Data[i] = Math.floor(peakData[i] * 255);
      }
      gl.texSubImage2D(
        gl.TEXTURE_2D, 0, 0, 0,
        this.textureWidth, 1,
        gl.LUMINANCE, gl.UNSIGNED_BYTE, uint8Data
      );
    }
    
    gl.bindTexture(gl.TEXTURE_2D, null);
  }

  /**
   * Resizes the renderer
   */
  resize(width: number, height: number): void {
    if (width <= 0 || height <= 0) return;
    
    const dpr = Math.min(window.devicePixelRatio || 1, this.isWebGL2 ? 2 : 1.5);
    this.width = width;
    this.height = height;
    
    if (this.canvas) {
      this.canvas.width = Math.floor(width * dpr);
      this.canvas.height = Math.floor(height * dpr);
    }
    
    this.gl?.viewport(0, 0, Math.floor(width * dpr), Math.floor(height * dpr));
  }

  /**
   * Renders the main scrolling waveform
   */
  renderWaveform(state: DJWaveformRenderState): void {
    if (!this.gl || !this.waveformProgram || !this.initialized || this.contextLost) return;
    
    const gl = this.gl;
    const prog = this.waveformProgram;
    const texture = state.deck === 'A' ? this.waveformTextureA : this.waveformTextureB;
    const deckColor = state.deck === 'A' ? this.options.deckAColor : this.options.deckBColor;
    
    // Clear
    gl.clearColor(0.071, 0.071, 0.071, 1.0);
    gl.clear(gl.COLOR_BUFFER_BIT);
    
    // Bind shader
    gl.useProgram(prog.program);
    
    // Set uniforms
    const dpr = Math.min(window.devicePixelRatio || 1, this.isWebGL2 ? 2 : 1.5);
    const hasPeaks = state.peaks && state.peaks.length > 0 && state.duration > 0;
    const position = hasPeaks ? state.position / state.duration : 0;
    const visibleRange = hasPeaks ? state.visibleSeconds / state.duration : 0.1;
    
    gl.uniform1i(this.getUniform(prog, 'u_waveformTex'), 0);
    gl.uniform1f(this.getUniform(prog, 'u_position'), position);
    gl.uniform1f(this.getUniform(prog, 'u_visibleRange'), visibleRange);
    gl.uniform2f(this.getUniform(prog, 'u_resolution'), this.width * dpr, this.height * dpr);
    gl.uniform1f(this.getUniform(prog, 'u_centerY'), 0.5);
    gl.uniform3fv(this.getUniform(prog, 'u_deckColor'), deckColor);
    gl.uniform1i(this.getUniform(prog, 'u_hasPeaks'), hasPeaks ? 1 : 0);
    
    // Bind texture
    gl.activeTexture(gl.TEXTURE0);
    gl.bindTexture(gl.TEXTURE_2D, texture);
    
    // Draw
    this.drawQuad();
    
    // Render overlays
    if (hasPeaks) {
      // Beat grid
      if (state.bpm > 0) {
        this.renderBeatGrid(state);
      }
      
      // Cue point
      if (state.cuePoint > 0) {
        this.renderCuePoint(state);
      }
      
      // Hot cues
      if (state.hotCues.length > 0) {
        this.renderHotCues(state);
      }
      
      // Playhead (always last, on top)
      this.renderPlayhead();
    }
  }

  /**
   * Renders the beat grid overlay
   */
  private renderBeatGrid(state: DJWaveformRenderState): void {
    if (!this.gl || !this.beatGridProgram) return;
    
    const gl = this.gl;
    const prog = this.beatGridProgram;
    
    gl.useProgram(prog.program);
    
    const dpr = Math.min(window.devicePixelRatio || 1, this.isWebGL2 ? 2 : 1.5);
    
    gl.uniform1f(this.getUniform(prog, 'u_position'), state.position);
    gl.uniform1f(this.getUniform(prog, 'u_bpm'), state.bpm);
    gl.uniform1f(this.getUniform(prog, 'u_visibleSeconds'), state.visibleSeconds);
    gl.uniform1f(this.getUniform(prog, 'u_beatOffset'), 0);
    gl.uniform2f(this.getUniform(prog, 'u_resolution'), this.width * dpr, this.height * dpr);
    
    this.drawQuad();
  }

  /**
   * Renders the playhead indicator
   */
  private renderPlayhead(): void {
    if (!this.gl || !this.playheadProgram) return;
    
    const gl = this.gl;
    const prog = this.playheadProgram;
    
    gl.useProgram(prog.program);
    
    const dpr = Math.min(window.devicePixelRatio || 1, this.isWebGL2 ? 2 : 1.5);
    
    gl.uniform2f(this.getUniform(prog, 'u_resolution'), this.width * dpr, this.height * dpr);
    gl.uniform3fv(this.getUniform(prog, 'u_color'), this.options.playheadColor);
    gl.uniform1f(this.getUniform(prog, 'u_markerSize'), 8 * dpr);
    
    this.drawQuad();
  }

  /**
   * Renders the cue point marker
   */
  private renderCuePoint(state: DJWaveformRenderState): void {
    if (!this.gl || !this.cuePointProgram || state.cuePoint <= 0) return;
    
    const gl = this.gl;
    const prog = this.cuePointProgram;
    
    gl.useProgram(prog.program);
    
    const dpr = Math.min(window.devicePixelRatio || 1, this.isWebGL2 ? 2 : 1.5);
    const cuePos = state.cuePoint / state.duration;
    const visibleRange = state.visibleSeconds / state.duration;
    const position = state.position / state.duration;
    
    gl.uniform2f(this.getUniform(prog, 'u_resolution'), this.width * dpr, this.height * dpr);
    gl.uniform1f(this.getUniform(prog, 'u_cuePosition'), cuePos);
    gl.uniform1f(this.getUniform(prog, 'u_position'), position);
    gl.uniform1f(this.getUniform(prog, 'u_visibleRange'), visibleRange);
    gl.uniform3fv(this.getUniform(prog, 'u_color'), this.options.cueColor);
    gl.uniform1f(this.getUniform(prog, 'u_markerSize'), 6 * dpr);
    
    this.drawQuad();
  }

  /**
   * Renders hot cue markers
   */
  private renderHotCues(state: DJWaveformRenderState): void {
    if (!this.gl || !this.hotCueProgram || state.hotCues.length === 0) return;
    
    const gl = this.gl;
    const prog = this.hotCueProgram;
    
    gl.useProgram(prog.program);
    
    const dpr = Math.min(window.devicePixelRatio || 1, this.isWebGL2 ? 2 : 1.5);
    const visibleRange = state.visibleSeconds / state.duration;
    const position = state.position / state.duration;
    
    // Prepare hot cue data (position + color packed into vec4)
    const cueData = new Float32Array(32); // 8 cues × 4 floats
    const cueCount = Math.min(state.hotCues.length, 8);
    
    for (let i = 0; i < cueCount; i++) {
      const hc = state.hotCues[i];
      const idx = i * 4;
      cueData[idx] = hc.position / state.duration; // Normalized position
      // Parse color
      const rgb = this.parseColor(hc.color);
      cueData[idx + 1] = rgb[0];
      cueData[idx + 2] = rgb[1];
      cueData[idx + 3] = rgb[2];
    }
    
    gl.uniform2f(this.getUniform(prog, 'u_resolution'), this.width * dpr, this.height * dpr);
    gl.uniform1f(this.getUniform(prog, 'u_position'), position);
    gl.uniform1f(this.getUniform(prog, 'u_visibleRange'), visibleRange);
    gl.uniform1i(this.getUniform(prog, 'u_cueCount'), cueCount);
    gl.uniform1f(this.getUniform(prog, 'u_markerSize'), 5 * dpr);
    
    // Set hot cue array uniform
    const loc = this.getUniform(prog, 'u_cuePositions');
    if (loc) {
      gl.uniform4fv(loc, cueData);
    }
    
    this.drawQuad();
  }

  /**
   * Renders the overview strip
   */
  renderOverview(
    deckA: { peaks: number[] | null; position: number; duration: number } | null,
    deckB: { peaks: number[] | null; position: number; duration: number } | null
  ): void {
    if (!this.gl || !this.overviewProgram || !this.initialized || this.contextLost) return;
    
    const gl = this.gl;
    const prog = this.overviewProgram;
    
    // Clear
    gl.clearColor(0.102, 0.102, 0.102, 1.0);
    gl.clear(gl.COLOR_BUFFER_BIT);
    
    const dpr = Math.min(window.devicePixelRatio || 1, this.isWebGL2 ? 2 : 1.5);
    const halfWidth = this.width / 2;
    
    // Render Deck A (left half)
    if (deckA && deckA.peaks && deckA.duration > 0) {
      gl.useProgram(prog.program);
      gl.viewport(0, 0, Math.floor(halfWidth * dpr), Math.floor(this.height * dpr));
      
      gl.uniform1i(this.getUniform(prog, 'u_waveformTex'), 0);
      gl.uniform2f(this.getUniform(prog, 'u_resolution'), halfWidth * dpr, this.height * dpr);
      gl.uniform1f(this.getUniform(prog, 'u_position'), deckA.position / deckA.duration);
      gl.uniform3fv(this.getUniform(prog, 'u_deckColor'), this.options.deckAColor);
      gl.uniform1i(this.getUniform(prog, 'u_hasPeaks'), 1);
      
      gl.activeTexture(gl.TEXTURE0);
      gl.bindTexture(gl.TEXTURE_2D, this.waveformTextureA);
      
      this.drawQuad();
    }
    
    // Render Deck B (right half)
    if (deckB && deckB.peaks && deckB.duration > 0) {
      gl.useProgram(prog.program);
      gl.viewport(Math.floor(halfWidth * dpr), 0, Math.floor(halfWidth * dpr), Math.floor(this.height * dpr));
      
      gl.uniform1i(this.getUniform(prog, 'u_waveformTex'), 0);
      gl.uniform2f(this.getUniform(prog, 'u_resolution'), halfWidth * dpr, this.height * dpr);
      gl.uniform1f(this.getUniform(prog, 'u_position'), deckB.position / deckB.duration);
      gl.uniform3fv(this.getUniform(prog, 'u_deckColor'), this.options.deckBColor);
      gl.uniform1i(this.getUniform(prog, 'u_hasPeaks'), 1);
      
      gl.activeTexture(gl.TEXTURE0);
      gl.bindTexture(gl.TEXTURE_2D, this.waveformTextureB);
      
      this.drawQuad();
    }
    
    // Reset viewport
    gl.viewport(0, 0, Math.floor(this.width * dpr), Math.floor(this.height * dpr));
  }

  /**
   * Draws the fullscreen quad
   */
  private drawQuad(): void {
    const gl = this.gl!;
    
    if (this.isWebGL2 && this.quadVAO) {
      const gl2 = gl as WebGL2RenderingContext;
      gl2.bindVertexArray(this.quadVAO);
      gl.drawArrays(gl.TRIANGLES, 0, 6);
      gl2.bindVertexArray(null);
    } else {
      gl.bindBuffer(gl.ARRAY_BUFFER, this.quadBuffer);
      gl.enableVertexAttribArray(0);
      gl.vertexAttribPointer(0, 2, gl.FLOAT, false, 0, 0);
      gl.drawArrays(gl.TRIANGLES, 0, 6);
    }
  }

  /**
   * Parses a CSS color string to RGB [0-1] values
   */
  private parseColor(color: string): [number, number, number] {
    // Handle hex colors
    if (color.startsWith('#')) {
      const hex = color.slice(1);
      const r = parseInt(hex.slice(0, 2), 16) / 255;
      const g = parseInt(hex.slice(2, 4), 16) / 255;
      const b = parseInt(hex.slice(4, 6), 16) / 255;
      return [r, g, b];
    }
    
    // Default to green
    return [0.133, 0.773, 0.369]; // #22c55e
  }

  /**
   * Updates renderer options
   */
  setOptions(options: Partial<DJWebGLRendererOptions>): void {
    this.options = { ...this.options, ...options };
  }

  /**
   * Checks if the renderer is ready
   */
  isReady(): boolean {
    return this.initialized && !this.contextLost;
  }

  /**
   * Checks if WebGL context has been lost
   */
  isContextLost(): boolean {
    return this.contextLost;
  }

  /**
   * Gets WebGL version info
   */
  getInfo(): { webgl2: boolean; maxTextureSize: number; contextLost: boolean } {
    return {
      webgl2: this.isWebGL2,
      maxTextureSize: this.gl?.getParameter(this.gl.MAX_TEXTURE_SIZE) || 0,
      contextLost: this.contextLost,
    };
  }

  /**
   * Disposes all WebGL resources
   */
  dispose(): void {
    const gl = this.gl;
    
    // Remove context loss handlers
    if (this.canvas) {
      if (this.contextLostHandler) {
        this.canvas.removeEventListener('webglcontextlost', this.contextLostHandler);
      }
      if (this.contextRestoredHandler) {
        this.canvas.removeEventListener('webglcontextrestored', this.contextRestoredHandler);
      }
    }
    this.contextLostHandler = null;
    this.contextRestoredHandler = null;
    
    if (gl) {
      // Delete textures
      if (this.waveformTextureA) gl.deleteTexture(this.waveformTextureA);
      if (this.waveformTextureB) gl.deleteTexture(this.waveformTextureB);
      
      // Delete buffers
      if (this.quadBuffer) gl.deleteBuffer(this.quadBuffer);
      
      // Delete VAO
      if (this.isWebGL2 && this.quadVAO) {
        (gl as WebGL2RenderingContext).deleteVertexArray(this.quadVAO);
      }
      
      // Delete programs
      const programs = [
        this.waveformProgram,
        this.beatGridProgram,
        this.playheadProgram,
        this.hotCueProgram,
        this.overviewProgram,
        this.cuePointProgram,
      ];
      
      for (const prog of programs) {
        if (prog) gl.deleteProgram(prog.program);
      }
    }
    
    this.canvas = null;
    this.gl = null;
    this.initialized = false;
    this.contextLost = false;
    
    console.log('[DJWebGL] Disposed');
  }
}

export default DJWebGLRenderer;
