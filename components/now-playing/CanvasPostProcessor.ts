/**
 * CanvasPostProcessor
 * 
 * Handles advanced post-processing effects for the visualization engine.
 * Implements an offscreen rendering pipeline to support:
 * 1. Bloom/Glow (via downscaled blur pass)
 * 2. Motion Trails (via feedback loop)
 * 3. Chromatic Aberration (future)
 * 
 * Usage:
 * 1. Initialize in component
 * 2. Call resize() on size changes
 * 3. Get draw context via getDrawContext()
 * 4. Draw visualization to that context
 * 5. Call render() to composite to the main screen
 */
export class CanvasPostProcessor {
    width: number = 0;
    height: number = 0;
    
    // Offscreen buffers
    private sceneCanvas: HTMLCanvasElement;
    private sceneCtx: CanvasRenderingContext2D;
    
    private bloomCanvas: HTMLCanvasElement;
    private bloomCtx: CanvasRenderingContext2D;
    
    constructor() {
        this.sceneCanvas = document.createElement('canvas');
        const sceneCtx = this.sceneCanvas.getContext('2d', { alpha: true });
        if (!sceneCtx) throw new Error('Could not create scene context');
        this.sceneCtx = sceneCtx;
        
        this.bloomCanvas = document.createElement('canvas');
        const bloomCtx = this.bloomCanvas.getContext('2d', { alpha: true });
        if (!bloomCtx) throw new Error('Could not create bloom context');
        this.bloomCtx = bloomCtx;
    }

    /**
     * Resizes internal buffers to match display size
     * @param width Display width
     * @param height Display height
     * @param dpr Device Pixel Ratio (default 1)
     */
    resize(width: number, height: number, dpr: number = 1) {
        const scaledWidth = width * dpr;
        const scaledHeight = height * dpr;
        
        if (this.width === scaledWidth && this.height === scaledHeight) return;
        
        this.width = scaledWidth;
        this.height = scaledHeight;
        
        // Scene canvas matches display resolution
        this.sceneCanvas.width = scaledWidth;
        this.sceneCanvas.height = scaledHeight;
        this.sceneCtx.scale(dpr, dpr);
        
        // Bloom canvas is 1/4 resolution for performance and softer blur
        // We don't scale context here because we just draw the image scaled
        this.bloomCanvas.width = scaledWidth / 4;
        this.bloomCanvas.height = scaledHeight / 4;
    }

    /**
     * Returns the context to draw the raw visualization onto
     */
    getDrawContext(): CanvasRenderingContext2D {
        return this.sceneCtx;
    }

    /**
     * Clears the scene buffer.
     * Use this or fade() at the start of a frame.
     */
    clear() {
        // We need to clear based on the logical size (since we scaled the context)
        // But clearRect uses transformed coordinates.
        // To be safe, we can reset transform or just clear a huge area.
        // Since we know the logical size (width/dpr), we can use that.
        // Or simpler: use the raw canvas width/height with reset transform.
        
        this.sceneCtx.save();
        this.sceneCtx.setTransform(1, 0, 0, 1, 0, 0);
        this.sceneCtx.clearRect(0, 0, this.sceneCanvas.width, this.sceneCanvas.height);
        this.sceneCtx.restore();
    }

    /**
     * Fades the scene buffer to create trails.
     * @param amount Fade amount (0.0 to 1.0). 0.1 means long trails, 0.9 means short trails.
     */
    fade(amount: number) {
        this.sceneCtx.save();
        this.sceneCtx.setTransform(1, 0, 0, 1, 0, 0);
        this.sceneCtx.fillStyle = `rgba(0, 0, 0, ${amount})`;
        this.sceneCtx.fillRect(0, 0, this.sceneCanvas.width, this.sceneCanvas.height);
        this.sceneCtx.restore();
    }

    /**
     * Composites the scene and effects onto the destination canvas
     * @param destCtx The main canvas context to render to
     * @param options Rendering options
     */
    render(destCtx: CanvasRenderingContext2D, options: {
        enableBloom?: boolean;
        bloomIntensity?: number;
        bloomRadius?: number;
    } = {}) {
        const { 
            enableBloom = true, 
            bloomIntensity = 1.5,
            bloomRadius = 20 
        } = options;

        // 1. Clear Destination
        // We assume destCtx is already scaled or we draw to it in screen space?
        // Usually destCtx passed from requestAnimationFrame is already set up.
        // But wait, in AlbumArtVisualizer, we handle scaling manually.
        // Let's assume destCtx is ready to receive the full resolution image.
        
        // We'll draw using setTransform(1,0,0,1,0,0) to ensure 1:1 pixel mapping
        destCtx.save();
        destCtx.setTransform(1, 0, 0, 1, 0, 0);
        destCtx.clearRect(0, 0, this.width, this.height);

        // 2. Draw Main Scene
        destCtx.drawImage(this.sceneCanvas, 0, 0);

        // 3. Apply Bloom
        if (enableBloom) {
            // Draw scene to bloom canvas (downscaled)
            this.bloomCtx.save();
            this.bloomCtx.filter = `blur(${bloomRadius / 4}px)`; // Adjust blur for downscale
            // Clear bloom canvas first? Not strictly necessary if we cover it, but good practice
            this.bloomCtx.clearRect(0, 0, this.bloomCanvas.width, this.bloomCanvas.height);
            
            // Draw scene onto bloom canvas
            this.bloomCtx.drawImage(
                this.sceneCanvas, 
                0, 0, this.sceneCanvas.width, this.sceneCanvas.height,
                0, 0, this.bloomCanvas.width, this.bloomCanvas.height
            );
            this.bloomCtx.restore();

            // Composite bloom onto destination
            destCtx.globalCompositeOperation = 'screen'; // 'screen' is better than 'lighter' for bloom
            destCtx.globalAlpha = bloomIntensity;
            
            // Draw the blurred bloom texture stretched back up
            destCtx.drawImage(
                this.bloomCanvas,
                0, 0, this.bloomCanvas.width, this.bloomCanvas.height,
                0, 0, this.width, this.height
            );
            
            // Reset
            destCtx.globalAlpha = 1.0;
            destCtx.globalCompositeOperation = 'source-over';
        }

        destCtx.restore();
    }
}
