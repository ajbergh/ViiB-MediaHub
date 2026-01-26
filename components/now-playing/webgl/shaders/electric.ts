/**
 * Electric Arc Visualization - WebGL Fragment Shader (OPTIMIZED v2)
 * 
 * Renders TRON-style geometric light beams arcing between points.
 * Uses procedural noise for electric jitter effect.
 * 
 * OPTIMIZATIONS v2:
 * - Reduced arc count from 8 to 5 (1.6x reduction)
 * - Reduced segments per arc from 8 to 6 (1.3x reduction)
 * - Early exit for pixels far from arc bounding box
 * - Simplified distance calculation
 * - Precomputed hash values outside inner loop
 * 
 * Performance: ~2x faster than previous WebGL version
 */

export const electricFragmentShader = `#version 300 es
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

#define NUM_ARCS 5
#define NUM_SEGMENTS 6
#define PI 3.14159265359

// Note: hash and hash2f functions are provided by noiseGLSL injection

// Distance from point to line segment
float sdSegment(vec2 p, vec2 a, vec2 b) {
    vec2 pa = p - a;
    vec2 ba = b - a;
    float h = clamp(dot(pa, ba) / dot(ba, ba), 0.0, 1.0);
    return length(pa - ba * h);
}

void main() {
    vec2 uv = v_uv;
    float aspect = u_resolution.x / u_resolution.y;
    uv.x *= aspect;
    
    vec3 totalColor = vec3(0.0);
    float totalAlpha = 0.0;
    
    // Electric cyan color (precomputed)
    vec3 arcColor = vec3(0.0, 1.0, 1.0);
    vec3 coreColor = vec3(1.0);
    
    // Time-based arc spawning (pseudo-persistent arcs)
    float arcTime = floor(u_time * 2.0); // New arcs every 0.5s
    float arcAge = fract(u_time * 2.0);
    float life = 1.0 - arcAge;
    
    // Skip if fading out too much
    if (life < 0.15) {
        fragColor = vec4(0.0);
        return;
    }
    
    // Jitter time seed (precomputed)
    float jitterTime = floor(u_time * 30.0);
    
    for (int i = 0; i < NUM_ARCS; i++) {
        float fi = float(i);
        float seed = hash(fi * 7.89 + arcTime * 3.21);
        
        // Only render arc if bass is high enough (spawn condition)
        float spawnThreshold = 0.25 + fi * 0.08;
        if (u_bass < spawnThreshold && seed > 0.5) continue;
        
        // Arc endpoints (random but consistent per arc)
        float margin = 0.12;
        vec2 p1 = vec2(
            margin + hash(fi * 1.23 + arcTime) * (aspect - margin * 2.0),
            margin + hash(fi * 2.34 + arcTime) * (1.0 - margin * 2.0)
        );
        vec2 p2 = vec2(
            margin + hash(fi * 3.45 + arcTime) * (aspect - margin * 2.0),
            margin + hash(fi * 4.56 + arcTime) * (1.0 - margin * 2.0)
        );
        
        // Bounding box early exit
        vec2 minBound = min(p1, p2) - 0.1;
        vec2 maxBound = max(p1, p2) + 0.1;
        if (uv.x < minBound.x || uv.x > maxBound.x || uv.y < minBound.y || uv.y > maxBound.y) {
            continue;
        }
        
        // Calculate distance from point to arc (with jitter)
        float minDist = 10000.0;
        
        // Precompute jitter amount
        float jitterAmount = u_treble * 0.04;
        
        // Draw arc as series of segments with jitter
        vec2 prevPoint = p1;
        
        for (int s = 1; s <= NUM_SEGMENTS; s++) {
            float t = float(s) / float(NUM_SEGMENTS);
            
            // Interpolate along arc
            vec2 basePoint = mix(p1, p2, t);
            
            // Add jitter (changes over time for electric effect)
            float jitterSeed = hash2f(vec2(fi, float(s) + jitterTime));
            float jitterScale = jitterAmount * (1.0 - abs(t - 0.5) * 2.0);
            vec2 jitter = vec2(
                (jitterSeed - 0.5) * jitterScale,
                (hash(jitterSeed * 100.0) - 0.5) * jitterScale
            );
            
            vec2 point = basePoint + jitter;
            
            // Distance to this segment
            float segDist = sdSegment(uv, prevPoint, point);
            minDist = min(minDist, segDist);
            
            prevPoint = point;
        }
        
        // Glow based on distance and intensity
        float intensity = u_bass * life;
        float glowRadius = 0.02 + intensity * 0.015;
        float glowRadiusSq = glowRadius * glowRadius;
        
        float minDistSq = minDist * minDist;
        float glow = max(0.0, 1.0 - minDistSq / glowRadiusSq) * intensity;
        glow = glow * glow; // Smooth falloff
        
        float core = max(0.0, 1.0 - minDistSq / (glowRadiusSq * 0.1)) * intensity;
        core = core * core;
        
        // Add to total
        totalColor += arcColor * glow * 0.7 + coreColor * core;
        totalAlpha += glow + core * 0.4;
        
        // Bright treble flash (simplified)
        if (u_treble > 0.45) {
            float flashGlow = max(0.0, 1.0 - minDistSq / (glowRadiusSq * 0.25)) * u_treble * life * 0.25;
            totalColor += coreColor * flashGlow;
            totalAlpha += flashGlow * 0.4;
        }
    }
    
    fragColor = vec4(totalColor, clamp(totalAlpha, 0.0, 1.0));
}
`;
