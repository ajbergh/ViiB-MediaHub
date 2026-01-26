/**
 * Spectrum Visualization - WebGL Fragment Shader
 * 
 * Renders circular frequency bars radiating from center (sun-burst effect).
 * Uses SDF for bar shapes with analytical glow.
 * 
 * Performance: ~3-5x faster than Canvas 2D version
 */

export const spectrumFragmentShader = `#version 300 es
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

#define NUM_BARS 64
#define PI 3.14159265359
#define TWO_PI 6.28318530718

void main() {
    // Center UV coordinates
    vec2 center = vec2(0.5);
    vec2 uv = v_uv - center;
    
    // Correct for aspect ratio
    float aspect = u_resolution.x / u_resolution.y;
    uv.x *= aspect;
    
    // Polar coordinates
    float dist = length(uv);
    float angle = atan(uv.y, uv.x);
    
    // Normalize angle to 0-1
    float normalizedAngle = (angle + PI) / TWO_PI;
    
    // Calculate which bar we're near
    float barWidth = 1.0 / float(NUM_BARS);
    float barIndex = floor(normalizedAngle * float(NUM_BARS));
    float barCenter = (barIndex + 0.5) * barWidth;
    
    // Distance from bar center (angular)
    float angularDist = abs(normalizedAngle - barCenter);
    angularDist = min(angularDist, 1.0 - angularDist); // Handle wrap-around
    
    // Sample frequency for this bar
    float freqIndex = barIndex / float(NUM_BARS) * 0.5; // Use lower 50% of spectrum
    float freqValue = texture(u_frequencyTexture, vec2(freqIndex, 0.5)).r;
    
    // Bar geometry
    float innerRadius = 0.2 + u_bass * 0.05;
    float maxBarHeight = 0.25;
    float barHeight = freqValue * maxBarHeight + 0.02;
    float outerRadius = innerRadius + barHeight;
    
    // Bar half-width in angular space
    float barHalfWidth = barWidth * 0.35;
    
    // SDF for bar
    float radialDist = 0.0;
    if (dist < innerRadius) {
        radialDist = innerRadius - dist;
    } else if (dist > outerRadius) {
        radialDist = dist - outerRadius;
    }
    
    float angularDistFromBar = max(0.0, angularDist - barHalfWidth);
    float angularDistPixels = angularDistFromBar * dist * TWO_PI;
    
    float barDist = max(radialDist, angularDistPixels);
    
    // Glow based on frequency value
    float glowIntensity = 0.5 + freqValue * 0.5;
    float glow = exp(-barDist * 30.0) * glowIntensity;
    float core = exp(-barDist * 80.0);
    
    // Color based on bar position (hue rotation)
    float hue = barIndex / float(NUM_BARS);
    vec3 barColor = mix(u_color, u_accentColor, hue);
    
    // Brightness boost based on amplitude
    barColor = mix(barColor, vec3(1.0), freqValue * 0.3);
    
    // Inner circle gradient
    float innerGlow = 0.0;
    if (dist < innerRadius * 1.2) {
        float innerDist = 1.0 - (dist / (innerRadius * 1.2));
        innerGlow = innerDist * innerDist * 0.2 * (0.5 + u_bass * 0.5);
    }
    
    // Combine
    vec3 color = barColor * glow + vec3(1.0) * core * 0.5 + u_color * innerGlow;
    float alpha = glow + core * 0.5 + innerGlow;
    
    fragColor = vec4(color, clamp(alpha, 0.0, 1.0));
}
`;
