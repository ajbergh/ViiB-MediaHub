/**
 * SpriteAtlas
 * 
 * Manages a shared texture atlas for sprite-based rendering.
 * Provides pre-generated glow, flare, and particle sprites as a single GPU texture.
 * 
 * Features:
 * - Procedurally generated sprites (glow, flare, ring, soft particle)
 * - Single texture atlas for efficient batched rendering
 * - UV coordinate lookup for each sprite type
 * - Configurable sprite sizes
 * 
 * Atlas Layout (512x512):
 * ┌─────────┬─────────┐
 * │  Glow   │  Flare  │
 * │ (256x)  │ (256x)  │
 * ├─────────┼─────────┤
 * │  Ring   │  Soft   │
 * │ (256x)  │ (256x)  │
 * └─────────┴─────────┘
 * 
 * @module SpriteAtlas
 */

export interface SpriteUV {
    u: number;
    v: number;
    width: number;
    height: number;
}

export type SpriteType = 'glow' | 'flare' | 'ring' | 'soft';

export class SpriteAtlas {
    private gl: WebGL2RenderingContext | WebGLRenderingContext;
    private texture: WebGLTexture | null = null;
    private atlasSize: number = 512;
    private spriteSize: number = 256;
    
    // UV coordinates for each sprite (normalized 0-1)
    private spriteUVs: Map<SpriteType, SpriteUV> = new Map([
        ['glow', { u: 0, v: 0, width: 0.5, height: 0.5 }],
        ['flare', { u: 0.5, v: 0, width: 0.5, height: 0.5 }],
        ['ring', { u: 0, v: 0.5, width: 0.5, height: 0.5 }],
        ['soft', { u: 0.5, v: 0.5, width: 0.5, height: 0.5 }]
    ]);

    constructor(gl: WebGL2RenderingContext | WebGLRenderingContext) {
        this.gl = gl;
        this.createAtlas();
    }

    /**
     * Creates the sprite atlas texture
     */
    private createAtlas(): void {
        const { gl } = this;
        
        // Create offscreen canvas for atlas generation
        const canvas = document.createElement('canvas');
        canvas.width = this.atlasSize;
        canvas.height = this.atlasSize;
        const ctx = canvas.getContext('2d')!;
        
        // Clear with transparent black
        ctx.clearRect(0, 0, this.atlasSize, this.atlasSize);
        
        // Generate sprites
        this.drawGlowSprite(ctx, 0, 0, this.spriteSize);
        this.drawFlareSprite(ctx, this.spriteSize, 0, this.spriteSize);
        this.drawRingSprite(ctx, 0, this.spriteSize, this.spriteSize);
        this.drawSoftSprite(ctx, this.spriteSize, this.spriteSize, this.spriteSize);
        
        // Create WebGL texture
        this.texture = gl.createTexture();
        gl.bindTexture(gl.TEXTURE_2D, this.texture);
        gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR_MIPMAP_LINEAR);
        gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR);
        gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE);
        gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE);
        
        // Upload canvas to texture
        gl.texImage2D(gl.TEXTURE_2D, 0, gl.RGBA, gl.RGBA, gl.UNSIGNED_BYTE, canvas);
        gl.generateMipmap(gl.TEXTURE_2D);
        
        gl.bindTexture(gl.TEXTURE_2D, null);
    }

    /**
     * Draws a soft radial glow sprite
     */
    private drawGlowSprite(ctx: CanvasRenderingContext2D, x: number, y: number, size: number): void {
        const center = size / 2;
        const gradient = ctx.createRadialGradient(
            x + center, y + center, 0,
            x + center, y + center, center
        );
        
        gradient.addColorStop(0, 'rgba(255, 255, 255, 1)');
        gradient.addColorStop(0.2, 'rgba(255, 255, 255, 0.8)');
        gradient.addColorStop(0.5, 'rgba(255, 255, 255, 0.3)');
        gradient.addColorStop(0.8, 'rgba(255, 255, 255, 0.1)');
        gradient.addColorStop(1, 'rgba(255, 255, 255, 0)');
        
        ctx.fillStyle = gradient;
        ctx.fillRect(x, y, size, size);
    }

    /**
     * Draws a lens flare / starburst sprite
     */
    private drawFlareSprite(ctx: CanvasRenderingContext2D, x: number, y: number, size: number): void {
        const center = size / 2;
        
        // Central glow
        const gradient = ctx.createRadialGradient(
            x + center, y + center, 0,
            x + center, y + center, center * 0.7
        );
        gradient.addColorStop(0, 'rgba(255, 255, 255, 0.9)');
        gradient.addColorStop(0.4, 'rgba(255, 255, 255, 0.3)');
        gradient.addColorStop(1, 'rgba(255, 255, 255, 0)');
        
        ctx.fillStyle = gradient;
        ctx.fillRect(x, y, size, size);
        
        // Horizontal streak
        ctx.fillStyle = 'rgba(255, 255, 255, 0.6)';
        ctx.beginPath();
        ctx.ellipse(x + center, y + center, center * 0.9, center * 0.04, 0, 0, Math.PI * 2);
        ctx.fill();
        
        // Vertical streak (smaller)
        ctx.fillStyle = 'rgba(255, 255, 255, 0.4)';
        ctx.beginPath();
        ctx.ellipse(x + center, y + center, center * 0.03, center * 0.6, 0, 0, Math.PI * 2);
        ctx.fill();
    }

    /**
     * Draws a ring sprite with soft edges
     */
    private drawRingSprite(ctx: CanvasRenderingContext2D, x: number, y: number, size: number): void {
        const center = size / 2;
        const radius = size * 0.35;
        const thickness = size * 0.08;
        
        ctx.strokeStyle = 'rgba(255, 255, 255, 0.9)';
        ctx.lineWidth = thickness;
        ctx.shadowColor = 'rgba(255, 255, 255, 0.8)';
        ctx.shadowBlur = thickness * 2;
        
        ctx.beginPath();
        ctx.arc(x + center, y + center, radius, 0, Math.PI * 2);
        ctx.stroke();
        
        // Reset shadow
        ctx.shadowBlur = 0;
    }

    /**
     * Draws a soft particle sprite (gaussian-like falloff)
     */
    private drawSoftSprite(ctx: CanvasRenderingContext2D, x: number, y: number, size: number): void {
        const center = size / 2;
        const gradient = ctx.createRadialGradient(
            x + center, y + center, 0,
            x + center, y + center, center
        );
        
        // Gaussian-like falloff
        gradient.addColorStop(0, 'rgba(255, 255, 255, 1)');
        gradient.addColorStop(0.1, 'rgba(255, 255, 255, 0.9)');
        gradient.addColorStop(0.3, 'rgba(255, 255, 255, 0.5)');
        gradient.addColorStop(0.6, 'rgba(255, 255, 255, 0.15)');
        gradient.addColorStop(1, 'rgba(255, 255, 255, 0)');
        
        ctx.fillStyle = gradient;
        ctx.fillRect(x, y, size, size);
    }

    /**
     * Binds the sprite atlas texture to a texture unit
     */
    bind(unit: number = 0): void {
        const { gl } = this;
        gl.activeTexture(gl.TEXTURE0 + unit);
        gl.bindTexture(gl.TEXTURE_2D, this.texture);
    }

    /**
     * Gets UV coordinates for a sprite type
     */
    getUV(type: SpriteType): SpriteUV {
        return this.spriteUVs.get(type) ?? { u: 0, v: 0, width: 0.5, height: 0.5 };
    }

    /**
     * Gets all sprite UVs for shader upload
     */
    getAllUVs(): Record<SpriteType, SpriteUV> {
        return Object.fromEntries(this.spriteUVs) as Record<SpriteType, SpriteUV>;
    }

    /**
     * Gets the texture
     */
    getTexture(): WebGLTexture | null {
        return this.texture;
    }

    /**
     * Disposes of GPU resources
     */
    dispose(): void {
        if (this.texture) {
            this.gl.deleteTexture(this.texture);
            this.texture = null;
        }
    }
}
