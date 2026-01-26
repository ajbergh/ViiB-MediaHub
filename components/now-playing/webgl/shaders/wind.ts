/**
 * Wind Field Visualization - WebGL Fragment Shader (OPTIMIZED v2)
 * 
 * Renders flowing particle wind effect with bass intensity and treble sparkles.
 * Uses procedural particle simulation with turbulence.
 * 
 * OPTIMIZATIONS v2:
 * - Reduced particle count from 100 to 50 (2x reduction)
 * - Early exit for pixels far from particles (distance squared check)
 * - Removed flow lines (minimal visual impact, high cost)
 * - Simplified sparkle calculation
 * - Precomputed constants outside loop
 * 
 * Performance: ~2-3x faster than previous WebGL version
 */

export const windFragmentShader = `#version 300 es
precision mediump float;

uniform vec2 u_resolution;
uniform float u_time;
uniform vec3 u_color;
uniform vec3 u_accentColor;
uniform sampler2D u_frequencyTexture;
uniform sampler2D u_waveformTexture;
uniform float u_bass;
uniform float u_mid;
uniform float u_treble;

in vec2 v_uv;
out vec4 fragColor;

#define NUM_PARTICLES 50
#define PI 3.14159265359

// Note: hash and hash2f functions are provided by noiseGLSL injection

void main() {
    vec2 uv = v_uv;
    float aspect = u_resolution.x / u_resolution.y;
    uv.x *= aspect;
    
    vec3 totalColor = vec3(0.0);
    float totalAlpha = 0.0;
    
    // Wind particle color (precomputed)
    vec3 windColor = vec3(0.7, 0.85, 1.0);
    vec3 sparkleColor = vec3(1.0);
    
    // Wind direction varies over time (precomputed)
    float windAngle = sin(u_time * 0.2) * PI * 0.2;
    float cosWind = cos(windAngle);
    float sinWind = sin(windAngle);
    float windSpeed = 1.0 + u_bass * 2.0;
    
    // Maximum glow distance squared for early exit
    float maxGlowDistSq = 0.002;
    
    // Base intensity factor
    float baseBrightness = 0.5 + u_bass * 0.5;
    
    // Treble sparkle threshold
    bool enableSparkles = u_treble > 0.4;
    float sparkleTimeSeed = floor(u_time * 10.0);
    
    for (int i = 0; i < NUM_PARTICLES; i++) {
        float fi = float(i);
        
        // Seed for this particle (precomputed hash)
        float seed = hash(fi * 12.9898);
        float seed2 = hash(fi * 78.233);
        
        // Base velocity
        float baseVx = 1.0 + seed * 2.0;
        float baseVy = (seed2 - 0.5) * 0.5;
        
        // Apply wind direction (precomputed cos/sin)
        vec2 velocity = vec2(
            baseVx * cosWind - baseVy * sinWind,
            baseVx * sinWind + baseVy * cosWind
        ) * windSpeed;
        
        // Starting position (wraps around)
        float startX = hash(fi * 1.234);
        float startY = hash(fi * 2.345);
        
        // Current position with time
        vec2 pos = vec2(startX, startY) + velocity * u_time * 0.05;
        
        // Add sinusoidal drift (simplified)
        pos.y += sin(pos.x * 5.0 + u_time * 0.5) * 0.03;
        
        // Wrap position
        pos.x = mod(pos.x * aspect, aspect);
        pos.y = mod(pos.y, 1.0);
        
        // Distance from this pixel to particle (squared for early exit)
        vec2 delta = uv - pos;
        float pDistSq = dot(delta, delta);
        
        // Early exit if too far
        if (pDistSq > maxGlowDistSq) continue;
        
        // Particle properties
        float size = 1.0 + seed * 2.0;
        float opacity = 0.3 + seed2 * 0.4;
        
        // Particle glow (simplified, no exp)
        float glowSize = 0.005 + size * 0.003;
        float glowSizeSq = glowSize * glowSize;
        float glow = max(0.0, 1.0 - pDistSq / glowSizeSq) * opacity * baseBrightness;
        
        // Sparkle on treble (simplified check)
        bool isSparkle = enableSparkles && hash2f(vec2(fi, sparkleTimeSeed)) < 0.15;
        
        // Color
        vec3 particleColor = isSparkle ? sparkleColor : windColor;
        float sparkleMultiplier = isSparkle ? 2.5 : 1.0;
        
        totalColor += particleColor * glow * sparkleMultiplier;
        totalAlpha += glow * 0.4;
    }
    
    fragColor = vec4(totalColor, clamp(totalAlpha, 0.0, 1.0));
}
`;
