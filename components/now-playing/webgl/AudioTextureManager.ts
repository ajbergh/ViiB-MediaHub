/**
 * AudioTextureManager
 * 
 * Manages audio data textures for WebGL visualizers.
 * Uploads frequency and waveform data as GPU textures for shader access.
 * 
 * Features:
 * - Dual texture system (frequency + waveform)
 * - Efficient per-frame texture updates
 * - Pre-computed energy bands (bass, mid, treble)
 * - Automatic texture unit management
 * 
 * Texture Format:
 * - Frequency: 256x1 R8 (normalized amplitude per frequency bin)
 * - Waveform: 256x1 R8 (time-domain samples, 128 = center)
 * 
 * @module AudioTextureManager
 */

export interface AudioEnergy {
    bass: number;      // 0-30 bins (0-1 normalized)
    mid: number;       // 30-150 bins (0-1 normalized)
    treble: number;    // 150-300 bins (0-1 normalized)
    overall: number;   // Overall energy
}

export class AudioTextureManager {
    private gl: WebGL2RenderingContext | WebGLRenderingContext;
    private frequencyTexture: WebGLTexture | null = null;
    private waveformTexture: WebGLTexture | null = null;
    private textureWidth: number = 256;
    
    // Pre-allocated typed arrays for texture upload
    private frequencyData: Uint8Array;
    private waveformData: Uint8Array;
    
    // Cached energy values
    private energy: AudioEnergy = {
        bass: 0,
        mid: 0,
        treble: 0,
        overall: 0
    };

    constructor(gl: WebGL2RenderingContext | WebGLRenderingContext) {
        this.gl = gl;
        this.frequencyData = new Uint8Array(this.textureWidth);
        this.waveformData = new Uint8Array(this.textureWidth);
        
        this.createTextures();
    }

    /**
     * Creates the frequency and waveform textures
     */
    private createTextures(): void {
        const { gl } = this;
        
        // Create frequency texture
        this.frequencyTexture = gl.createTexture();
        gl.bindTexture(gl.TEXTURE_2D, this.frequencyTexture);
        gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR);
        gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR);
        gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE);
        gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE);
        gl.texImage2D(gl.TEXTURE_2D, 0, gl.LUMINANCE, this.textureWidth, 1, 0, gl.LUMINANCE, gl.UNSIGNED_BYTE, null);
        
        // Create waveform texture
        this.waveformTexture = gl.createTexture();
        gl.bindTexture(gl.TEXTURE_2D, this.waveformTexture);
        gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR);
        gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR);
        gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE);
        gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE);
        gl.texImage2D(gl.TEXTURE_2D, 0, gl.LUMINANCE, this.textureWidth, 1, 0, gl.LUMINANCE, gl.UNSIGNED_BYTE, null);
        
        gl.bindTexture(gl.TEXTURE_2D, null);
    }

    /**
     * Updates audio textures with new data from the analyser
     * 
     * @param frequencyData - Byte frequency data from AnalyserNode
     * @param waveformData - Byte time domain data from AnalyserNode
     */
    update(frequencyData: Uint8Array, waveformData: Uint8Array): void {
        const { gl } = this;
        
        // Downsample/copy to our texture-sized arrays
        const freqStep = Math.floor(frequencyData.length / this.textureWidth);
        const waveStep = Math.floor(waveformData.length / this.textureWidth);
        
        let bassSum = 0, midSum = 0, trebleSum = 0, overallSum = 0;
        const bassEnd = 30;
        const midEnd = 150;
        const trebleEnd = Math.min(300, frequencyData.length);
        
        for (let i = 0; i < this.textureWidth; i++) {
            // Average nearby samples for smoother texture
            let freqSum = 0;
            for (let j = 0; j < freqStep; j++) {
                const idx = i * freqStep + j;
                if (idx < frequencyData.length) {
                    freqSum += frequencyData[idx];
                }
            }
            this.frequencyData[i] = Math.round(freqSum / freqStep);
            
            let waveSum = 0;
            for (let j = 0; j < waveStep; j++) {
                const idx = i * waveStep + j;
                if (idx < waveformData.length) {
                    waveSum += waveformData[idx];
                }
            }
            this.waveformData[i] = Math.round(waveSum / waveStep);
            
            // Calculate energy bands using original data indices
            const origIdx = i * freqStep;
            if (origIdx < bassEnd) {
                bassSum += frequencyData[origIdx] || 0;
            } else if (origIdx < midEnd) {
                midSum += frequencyData[origIdx] || 0;
            } else if (origIdx < trebleEnd) {
                trebleSum += frequencyData[origIdx] || 0;
            }
            overallSum += frequencyData[origIdx] || 0;
        }
        
        // Normalize energy values
        this.energy = {
            bass: (bassSum / bassEnd) / 255,
            mid: (midSum / (midEnd - 30)) / 255,
            treble: (trebleSum / (trebleEnd - midEnd)) / 255,
            overall: (overallSum / this.textureWidth) / 255
        };
        
        // Upload to GPU
        gl.bindTexture(gl.TEXTURE_2D, this.frequencyTexture);
        gl.texSubImage2D(gl.TEXTURE_2D, 0, 0, 0, this.textureWidth, 1, gl.LUMINANCE, gl.UNSIGNED_BYTE, this.frequencyData);
        
        gl.bindTexture(gl.TEXTURE_2D, this.waveformTexture);
        gl.texSubImage2D(gl.TEXTURE_2D, 0, 0, 0, this.textureWidth, 1, gl.LUMINANCE, gl.UNSIGNED_BYTE, this.waveformData);
        
        gl.bindTexture(gl.TEXTURE_2D, null);
    }

    /**
     * Binds audio textures to specified texture units
     * 
     * @param frequencyUnit - Texture unit for frequency data (default: 0)
     * @param waveformUnit - Texture unit for waveform data (default: 1)
     */
    bind(frequencyUnit: number = 0, waveformUnit: number = 1): void {
        const { gl } = this;
        
        gl.activeTexture(gl.TEXTURE0 + frequencyUnit);
        gl.bindTexture(gl.TEXTURE_2D, this.frequencyTexture);
        
        gl.activeTexture(gl.TEXTURE0 + waveformUnit);
        gl.bindTexture(gl.TEXTURE_2D, this.waveformTexture);
    }

    /**
     * Gets the current energy values
     */
    getEnergy(): AudioEnergy {
        return { ...this.energy };
    }

    /**
     * Gets the frequency texture
     */
    getFrequencyTexture(): WebGLTexture | null {
        return this.frequencyTexture;
    }

    /**
     * Gets the waveform texture
     */
    getWaveformTexture(): WebGLTexture | null {
        return this.waveformTexture;
    }

    /**
     * Disposes of GPU resources
     */
    dispose(): void {
        const { gl } = this;
        
        if (this.frequencyTexture) {
            gl.deleteTexture(this.frequencyTexture);
            this.frequencyTexture = null;
        }
        
        if (this.waveformTexture) {
            gl.deleteTexture(this.waveformTexture);
            this.waveformTexture = null;
        }
    }
}
