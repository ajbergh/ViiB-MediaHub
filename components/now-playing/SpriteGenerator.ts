/**
 * SpriteGenerator
 * 
 * Utility to procedurally generate texture sprites for the visualization engine.
 * Using pre-rendered sprites (offscreen canvases) is significantly faster than
 * drawing complex gradients or paths every frame.
 */
export const SpriteGenerator = {
    /**
     * Creates a soft radial glow sprite
     */
    createGlowSprite: (size: number, color: string = 'rgba(255, 255, 255, 1)'): HTMLCanvasElement => {
        const canvas = document.createElement('canvas');
        canvas.width = size;
        canvas.height = size;
        const ctx = canvas.getContext('2d')!;
        const center = size / 2;
        
        // Clear
        ctx.clearRect(0, 0, size, size);
        
        // Soft radial gradient
        const gradient = ctx.createRadialGradient(center, center, 0, center, center, center);
        
        // Parse color to handle alpha if needed, but simple string works for now
        // We assume color is a valid CSS color string
        
        gradient.addColorStop(0, color);
        gradient.addColorStop(0.2, color.replace('1)', '0.8)').replace('1)', '0.8)')); // Slightly softer core
        gradient.addColorStop(0.5, color.replace('1)', '0.2)').replace('1)', '0.2)')); // Fade
        gradient.addColorStop(1, 'rgba(0, 0, 0, 0)');
        
        ctx.fillStyle = gradient;
        ctx.fillRect(0, 0, size, size);
        
        return canvas;
    },
    
    /**
     * Creates a lens flare / starburst sprite
     */
    createFlareSprite: (size: number): HTMLCanvasElement => {
        const canvas = document.createElement('canvas');
        canvas.width = size;
        canvas.height = size;
        const ctx = canvas.getContext('2d')!;
        const center = size / 2;
        
        ctx.clearRect(0, 0, size, size);
        
        // 1. Central Glow
        const gradient = ctx.createRadialGradient(center, center, 0, center, center, center * 0.7);
        gradient.addColorStop(0, 'rgba(255, 255, 255, 0.9)');
        gradient.addColorStop(0.4, 'rgba(255, 255, 255, 0.2)');
        gradient.addColorStop(1, 'rgba(255, 255, 255, 0)');
        ctx.fillStyle = gradient;
        ctx.fillRect(0, 0, size, size);
        
        // 2. Horizontal Streak
        ctx.fillStyle = 'rgba(255, 255, 255, 0.8)';
        ctx.beginPath();
        ctx.ellipse(center, center, center * 0.9, center * 0.05, 0, 0, Math.PI * 2);
        ctx.fill();
        
        // 3. Vertical Streak (smaller)
        ctx.fillStyle = 'rgba(255, 255, 255, 0.6)';
        ctx.beginPath();
        ctx.ellipse(center, center, center * 0.05, center * 0.6, 0, 0, Math.PI * 2);
        ctx.fill();
        
        return canvas;
    },

    /**
     * Creates a ring sprite with soft edges
     */
    createRingSprite: (size: number, thickness: number, color: string = 'white'): HTMLCanvasElement => {
        const canvas = document.createElement('canvas');
        canvas.width = size;
        canvas.height = size;
        const ctx = canvas.getContext('2d')!;
        const center = size / 2;
        const radius = size / 2 - thickness;
        
        ctx.clearRect(0, 0, size, size);
        
        ctx.shadowColor = color;
        ctx.shadowBlur = thickness * 2;
        ctx.strokeStyle = color;
        ctx.lineWidth = thickness;
        
        ctx.beginPath();
        ctx.arc(center, center, radius, 0, Math.PI * 2);
        ctx.stroke();
        
        return canvas;
    },

    /**
     * Creates a soft cloud/smoke sprite
     */
    createCloudSprite: (size: number): HTMLCanvasElement => {
        const canvas = document.createElement('canvas');
        canvas.width = size;
        canvas.height = size;
        const ctx = canvas.getContext('2d')!;
        const center = size / 2;
        
        ctx.clearRect(0, 0, size, size);
        
        // Draw multiple soft puffs
        for (let i = 0; i < 8; i++) {
            const r = size * (0.2 + Math.random() * 0.3);
            const x = center + (Math.random() - 0.5) * size * 0.4;
            const y = center + (Math.random() - 0.5) * size * 0.4;
            
            const gradient = ctx.createRadialGradient(x, y, 0, x, y, r);
            gradient.addColorStop(0, 'rgba(255, 255, 255, 0.15)');
            gradient.addColorStop(1, 'rgba(255, 255, 255, 0)');
            
            ctx.fillStyle = gradient;
            ctx.beginPath();
            ctx.arc(x, y, r, 0, Math.PI * 2);
            ctx.fill();
        }
        
        return canvas;
    }
};
