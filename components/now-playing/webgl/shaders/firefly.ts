/**
 * Firefly Field Visualization - WebGL Fragment Shader
 * 
 * Renders drifting firefly particles with warm glow and gentle flicker.
 * Uses procedural noise for movement and flickering patterns.
 * 
 * Performance: ~3-5x faster than Canvas 2D version
 */

export const fireflyFragmentShader = `#version 300 es
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

#define NUM_FIREFLIES 40
#define PI 3.14159265359

// Note: hash and hash2f functions are provided by noiseGLSL injection

void main() {
    vec2 uv = v_uv;
    float aspect = u_resolution.x / u_resolution.y;
    
    vec3 totalColor = vec3(0.0);
    float totalAlpha = 0.0;
    
    // Warm firefly colors
    vec3 warmColor1 = vec3(1.0, 0.9, 0.6);   // Bright warm yellow
    vec3 warmColor2 = vec3(1.0, 0.78, 0.4);  // Orange
    vec3 warmColor3 = vec3(1.0, 0.7, 0.3);   // Deeper orange
    
    for (int i = 0; i < NUM_FIREFLIES; i++) {
        float fi = float(i);
        
        // Pseudo-random seed per firefly
        float seed = hash(fi * 12.9898);
        
        // Base position (wrapping movement)
        float speed = 0.02 + seed * 0.03;
        float px = hash(fi * 1.234) + u_time * speed * 0.3;
        float py = hash(fi * 2.345) + u_time * speed * 0.2;
        
        // Add drifting motion
        px += sin(u_time * 0.3 + fi * 0.7) * 0.1;
        py += cos(u_time * 0.25 + fi * 0.9) * 0.1;
        
        // Wrap position
        px = fract(px);
        py = fract(py);
        
        // Adjust for aspect ratio
        vec2 pos = vec2(px, py);
        vec2 diff = uv - pos;
        diff.x *= aspect;
        
        float dist = length(diff);
        
        // Flicker phase
        float phase = seed * PI * 2.0 + u_time * (1.0 + seed * 2.0);
        float flicker = sin(phase) * 0.5 + 0.5;
        flicker = flicker * flicker; // Sharper flicker
        
        // Brightness varies with mids
        float baseBrightness = 0.3 + seed * 0.4;
        float brightness = baseBrightness * (0.5 + u_mid * 0.3) * (0.4 + flicker * 0.6);
        
        // Occasional bright flash on treble peaks
        float flash = 0.0;
        if (u_treble > 0.5 && hash2f(vec2(fi, floor(u_time * 10.0))) < 0.1) {
            flash = 1.0;
        }
        brightness += flash * 0.5;
        
        // Glow size varies with flicker
        float glowSize = (0.02 + brightness * 0.04) * (1.0 + flash * 0.5);
        
        // Multi-layer glow
        float glow1 = exp(-dist * dist / (glowSize * glowSize * 2.0)) * brightness;
        float glow2 = exp(-dist * dist / (glowSize * glowSize * 8.0)) * brightness * 0.4;
        float core = exp(-dist * dist / (glowSize * glowSize * 0.3)) * brightness;
        
        // Color varies slightly per firefly
        vec3 fireflyColor = mix(warmColor1, mix(warmColor2, warmColor3, seed), seed * 0.5);
        
        // White core, colored glow
        vec3 contribution = vec3(1.0) * core + fireflyColor * (glow1 + glow2);
        
        totalColor += contribution;
        totalAlpha += glow1 + glow2 * 0.5 + core * 0.5;
    }
    
    // Clamp and output
    totalAlpha = clamp(totalAlpha, 0.0, 1.0);
    
    fragColor = vec4(totalColor, totalAlpha);
}
`;
