/**
 * Wave Visualization - WebGL Fragment Shader
 * 
 * Renders a smooth glowing waveform using SDF techniques.
 * The waveform is sampled from the audio texture and rendered
 * with analytical glow (no post-processing needed).
 * 
 * Performance: ~2-3x faster than Canvas 2D version
 */

export const waveFragmentShader = `#version 300 es
precision mediump float;

uniform vec2 u_resolution;
uniform float u_time;
uniform vec3 u_color;
uniform vec3 u_accentColor;
uniform sampler2D u_waveformTexture;
uniform sampler2D u_frequencyTexture;
uniform float u_bass;
uniform float u_mid;
uniform float u_treble;

in vec2 v_uv;
out vec4 fragColor;

void main() {
    vec2 uv = v_uv;
    
    // Sample waveform at this x position
    float waveValue = texture(u_waveformTexture, vec2(uv.x, 0.5)).r;
    
    // Convert from 0-1 to centered position (0.25 to 0.75 of screen height)
    float waveY = waveValue * 0.5 + 0.25;
    
    // Add some bass-driven vertical movement
    waveY += (u_bass - 0.5) * 0.1;
    
    // Distance from current pixel to wave
    float dist = abs(uv.y - waveY);
    
    // Analytical glow layers
    float core = exp(-dist * 150.0);           // Sharp white core
    float glow1 = exp(-dist * 40.0) * 0.8;     // Primary glow
    float glow2 = exp(-dist * 15.0) * 0.4;     // Outer glow
    float glow3 = exp(-dist * 5.0) * 0.15;     // Ambient spread
    
    // Color mixing
    vec3 coreColor = vec3(1.0);                // White core
    vec3 glowColor = u_color;                  // Primary color glow
    vec3 outerColor = mix(u_color, u_accentColor, 0.5); // Accent for outer
    
    // Combine layers
    vec3 color = coreColor * core +
                 glowColor * glow1 +
                 outerColor * glow2 +
                 u_accentColor * glow3;
    
    // Overall intensity based on audio energy
    float intensity = 0.7 + u_mid * 0.3;
    color *= intensity;
    
    // Alpha from total glow
    float alpha = core + glow1 + glow2 * 0.5 + glow3 * 0.25;
    alpha = clamp(alpha, 0.0, 1.0);
    
    fragColor = vec4(color, alpha);
}
`;
