/**
 * WebGLVisualizerRenderer
 * 
 * Main WebGL rendering engine for ViiB MediaHub audio visualizations.
 * Manages WebGL context, shader programs, audio textures, and per-mode rendering.
 * 
 * Architecture:
 * - WebGL2 primary with WebGL1 fallback
 * - Shared audio texture manager for all modes
 * - Per-mode shader programs loaded on demand
 * - Unified fullscreen quad for fragment-shader modes
 * - Instanced rendering for particle modes
 * 
 * @module WebGLVisualizerRenderer
 */

import { ShaderProgram, ShaderCache } from './ShaderProgram';
import { AudioTextureManager, AudioEnergy } from './AudioTextureManager';
import { SpriteAtlas } from './SpriteAtlas';
import { VisualizerMode } from '../../../types';

// Import all shaders from centralized index
import {
    FULLSCREEN_QUAD_VERTICES,
    commonVertexShader,
    commonVertexShaderWebGL1,
    noiseGLSL,
    sdfGLSL,
    audioGLSL,
    waveFragmentShader,
    spectrumFragmentShader,
    fireflyFragmentShader,
    auroraFragmentShader,
    electricFragmentShader,
    grassFragmentShader,
    flameFragmentShader,
    stardustFragmentShader,
    windFragmentShader,
    tunnelFragmentShader
} from './shaders';

export interface WebGLVisualizerOptions {
    /** Primary color for visualizations */
    color?: [number, number, number];
    /** Secondary/accent color */
    accentColor?: [number, number, number];
    /** Enable bloom post-processing */
    enableBloom?: boolean;
    /** Bloom intensity (0-2) */
    bloomIntensity?: number;
}

const DEFAULT_OPTIONS: Required<WebGLVisualizerOptions> = {
    color: [0.235, 0.812, 0.467],      // Playback green
    accentColor: [0.545, 0.361, 0.965], // Brand purple
    enableBloom: true,
    bloomIntensity: 1.2
};

export class WebGLVisualizerRenderer {
    private canvas: HTMLCanvasElement | null = null;
    private gl: WebGL2RenderingContext | WebGLRenderingContext | null = null;
    private isWebGL2: boolean = false;
    
    // Resource managers
    private shaderCache: ShaderCache | null = null;
    private audioManager: AudioTextureManager | null = null;
    private spriteAtlas: SpriteAtlas | null = null;
    
    // Geometry
    private quadVAO: WebGLVertexArrayObject | null = null;
    private quadBuffer: WebGLBuffer | null = null;
    
    // State
    private width: number = 0;
    private height: number = 0;
    private time: number = 0;
    private lastFrameTime: number = 0;
    private options: Required<WebGLVisualizerOptions>;
    
    // Audio data buffers (reusable)
    private frequencyData: Uint8Array = new Uint8Array(1024);
    private waveformData: Uint8Array = new Uint8Array(1024);
    
    // Initialization state
    private initialized: boolean = false;

    constructor(options: WebGLVisualizerOptions = {}) {
        this.options = { ...DEFAULT_OPTIONS, ...options };
    }

    /**
     * Initializes WebGL context and resources
     * 
     * @param canvas - Canvas element to render to
     * @returns true if initialization succeeded
     */
    init(canvas: HTMLCanvasElement): boolean {
        if (this.initialized) {
            console.warn('[WebGLVisualizer] Already initialized');
            return true;
        }
        
        this.canvas = canvas;
        
        // Try WebGL2 first, fall back to WebGL1
        let gl: WebGL2RenderingContext | WebGLRenderingContext | null = canvas.getContext('webgl2', {
            alpha: true,
            antialias: false,
            depth: false,
            stencil: false,
            premultipliedAlpha: true,
            preserveDrawingBuffer: false
        });
        
        if (gl) {
            this.isWebGL2 = true;
            console.log('[WebGLVisualizer] Using WebGL2');
        } else {
            gl = canvas.getContext('webgl', {
                alpha: true,
                antialias: false,
                depth: false,
                stencil: false,
                premultipliedAlpha: true
            });
            
            if (gl) {
                this.isWebGL2 = false;
                console.log('[WebGLVisualizer] Using WebGL1 fallback');
            }
        }
        
        if (!gl) {
            console.error('[WebGLVisualizer] WebGL not supported');
            return false;
        }
        
        this.gl = gl;
        
        try {
            // Initialize resource managers
            this.shaderCache = new ShaderCache(gl);
            this.audioManager = new AudioTextureManager(gl);
            this.spriteAtlas = new SpriteAtlas(gl);
            
            // Create fullscreen quad geometry
            this.createQuadGeometry();
            
            // Set initial viewport
            this.resize(canvas.clientWidth, canvas.clientHeight);
            
            // Configure GL state
            gl.enable(gl.BLEND);
            gl.blendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA);
            gl.disable(gl.DEPTH_TEST);
            gl.disable(gl.CULL_FACE);
            
            this.initialized = true;
            console.log('[WebGLVisualizer] Initialization complete');
            return true;
            
        } catch (error) {
            console.error('[WebGLVisualizer] Initialization failed:', error);
            this.dispose();
            return false;
        }
    }

    /**
     * Creates the fullscreen quad VAO/VBO
     */
    private createQuadGeometry(): void {
        const gl = this.gl!;
        
        // Create buffer
        this.quadBuffer = gl.createBuffer();
        gl.bindBuffer(gl.ARRAY_BUFFER, this.quadBuffer);
        gl.bufferData(gl.ARRAY_BUFFER, FULLSCREEN_QUAD_VERTICES, gl.STATIC_DRAW);
        
        // Create VAO for WebGL2
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
     * Updates audio data from analyser node
     */
    setAudioData(frequencyData: Uint8Array, waveformData: Uint8Array): void {
        // Store references for modes that need direct access
        this.frequencyData = frequencyData;
        this.waveformData = waveformData;
        
        // Upload to GPU textures
        this.audioManager?.update(frequencyData, waveformData);
    }

    /**
     * Gets current audio energy levels
     */
    getEnergy(): AudioEnergy {
        return this.audioManager?.getEnergy() ?? {
            bass: 0, mid: 0, treble: 0, overall: 0
        };
    }

    /**
     * Resizes the renderer to match canvas size
     * 
     * Performance optimization: DPR is capped at 1.5 to prevent
     * excessive pixel counts on HiDPI displays. The shaders are
     * designed to look good at this resolution while maintaining
     * 60fps on mid-range hardware.
     */
    resize(width: number, height: number): void {
        // Cap DPR at 1.5 for performance (shaders are pixel-heavy)
        const dpr = Math.min(window.devicePixelRatio || 1, 1.5);
        this.width = width;
        this.height = height;
        
        if (this.canvas) {
            this.canvas.width = Math.floor(width * dpr);
            this.canvas.height = Math.floor(height * dpr);
        }
        
        this.gl?.viewport(0, 0, Math.floor(width * dpr), Math.floor(height * dpr));
    }

    /**
     * Updates options at runtime
     */
    setOptions(options: Partial<WebGLVisualizerOptions>): void {
        this.options = { ...this.options, ...options };
    }

    /**
     * Renders a frame for the specified visualization mode
     */
    render(mode: VisualizerMode, timestamp: number): void {
        if (!this.gl || !this.initialized) return;
        
        // Update time
        const deltaTime = this.lastFrameTime ? timestamp - this.lastFrameTime : 16.67;
        this.lastFrameTime = timestamp;
        this.time += deltaTime;
        
        const gl = this.gl;
        
        // Clear
        gl.clearColor(0, 0, 0, 0);
        gl.clear(gl.COLOR_BUFFER_BIT);
        
        // Skip if OFF mode
        if (mode === 'OFF' || mode === 'MILKDROP') return;
        
        // Get or compile shader for this mode
        const shader = this.getShaderForMode(mode);
        if (!shader) return;
        
        // Bind quad geometry
        if (this.isWebGL2 && this.quadVAO) {
            (gl as WebGL2RenderingContext).bindVertexArray(this.quadVAO);
        } else {
            gl.bindBuffer(gl.ARRAY_BUFFER, this.quadBuffer);
            gl.enableVertexAttribArray(0);
            gl.vertexAttribPointer(0, 2, gl.FLOAT, false, 0, 0);
        }
        
        // Use shader
        shader.use();
        
        // Set common uniforms
        shader.setUniform2f('u_resolution', this.width, this.height);
        shader.setUniform1f('u_time', this.time / 1000);
        shader.setUniform3f('u_color', ...this.options.color);
        shader.setUniform3f('u_accentColor', ...this.options.accentColor);
        
        // Bind audio textures
        this.audioManager?.bind(0, 1);
        shader.setUniform1i('u_frequencyTexture', 0);
        shader.setUniform1i('u_waveformTexture', 1);
        
        // Set energy uniforms
        const energy = this.getEnergy();
        shader.setUniform1f('u_bass', energy.bass);
        shader.setUniform1f('u_mid', energy.mid);
        shader.setUniform1f('u_treble', energy.treble);
        
        // Draw fullscreen quad
        gl.drawArrays(gl.TRIANGLES, 0, 6);
        
        // Cleanup
        if (this.isWebGL2 && this.quadVAO) {
            (gl as WebGL2RenderingContext).bindVertexArray(null);
        }
    }

    /**
     * Gets or compiles shader program for a visualization mode
     */
    private getShaderForMode(mode: VisualizerMode): ShaderProgram | null {
        if (!this.shaderCache) return null;
        
        // Check cache first
        if (this.shaderCache.has(mode)) {
            return this.shaderCache.getProgram(mode, { vertex: '', fragment: '' });
        }
        
        // Get fragment shader for mode
        let fragmentSource = this.getFragmentShaderForMode(mode);
        if (!fragmentSource) {
            console.warn(`[WebGLVisualizer] No shader for mode: ${mode}`);
            return null;
        }
        
        // Inject utility functions
        fragmentSource = this.injectUtilities(fragmentSource);
        
        // Select vertex shader based on WebGL version
        const vertexSource = this.isWebGL2 ? commonVertexShader : commonVertexShaderWebGL1;
        
        try {
            return this.shaderCache.getProgram(mode, {
                vertex: vertexSource,
                fragment: fragmentSource
            });
        } catch (error) {
            console.error(`[WebGLVisualizer] Shader compilation failed for ${mode}:`, error);
            return null;
        }
    }

    /**
     * Gets fragment shader source for a mode
     */
    private getFragmentShaderForMode(mode: VisualizerMode): string | null {
        switch (mode) {
            case 'WAVE':
                return waveFragmentShader;
            case 'SPECTRUM':
                return spectrumFragmentShader;
            case 'FIREFLY_FIELD':
                return fireflyFragmentShader;
            case 'AURORA_RIBBON':
                return auroraFragmentShader;
            case 'ELECTRIC_ARC':
                return electricFragmentShader;
            case 'GRASS_OSCILLOSCOPE':
                return grassFragmentShader;
            case 'FLAME_SPECTRUM':
                return flameFragmentShader;
            case 'STARDUST_HALO':
                return stardustFragmentShader;
            case 'WIND_FIELD':
                return windFragmentShader;
            case 'TUNNEL_WAVEFORM':
                return tunnelFragmentShader;
            default:
                return null;
        }
    }

    /**
     * Injects utility GLSL code into fragment shader
     */
    private injectUtilities(fragmentSource: string): string {
        // Find the location after #version and precision declarations
        const lines = fragmentSource.split('\n');
        let insertIndex = 0;
        
        for (let i = 0; i < lines.length; i++) {
            const trimmed = lines[i].trim();
            if (trimmed.startsWith('#version') || 
                trimmed.startsWith('precision') ||
                trimmed === '') {
                insertIndex = i + 1;
            } else {
                break;
            }
        }
        
        // Insert utilities after declarations
        lines.splice(insertIndex, 0, 
            '// === Injected Utilities ===',
            noiseGLSL,
            sdfGLSL,
            audioGLSL,
            '// === End Utilities ==='
        );
        
        return lines.join('\n');
    }

    /**
     * Checks if WebGL is supported
     */
    static isSupported(): boolean {
        try {
            const canvas = document.createElement('canvas');
            return !!(canvas.getContext('webgl2') || canvas.getContext('webgl'));
        } catch {
            return false;
        }
    }

    /**
     * Gets WebGL version being used
     */
    getWebGLVersion(): number {
        return this.isWebGL2 ? 2 : 1;
    }

    /**
     * Checks if renderer is initialized
     */
    isInitialized(): boolean {
        return this.initialized;
    }

    /**
     * Disposes all GPU resources
     */
    dispose(): void {
        if (this.gl) {
            // Dispose managers
            this.shaderCache?.dispose();
            this.audioManager?.dispose();
            this.spriteAtlas?.dispose();
            
            // Delete geometry
            if (this.quadVAO && this.isWebGL2) {
                (this.gl as WebGL2RenderingContext).deleteVertexArray(this.quadVAO);
            }
            if (this.quadBuffer) {
                this.gl.deleteBuffer(this.quadBuffer);
            }
        }
        
        this.shaderCache = null;
        this.audioManager = null;
        this.spriteAtlas = null;
        this.quadVAO = null;
        this.quadBuffer = null;
        this.gl = null;
        this.canvas = null;
        this.initialized = false;
        
        console.log('[WebGLVisualizer] Disposed');
    }
}
