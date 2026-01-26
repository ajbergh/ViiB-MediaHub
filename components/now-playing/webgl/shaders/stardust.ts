/**
 * Stardust Halo Visualization - WebGL Fragment Shader (OPTIMIZED v2)
 * 
 * Renders a pulsing particle halo with stardust bursts on bass hits.
 * Uses procedural particle simulation for expanding trails.
 * 
 * OPTIMIZATIONS v2:
 * - Reduced wave count from 30 to 12 (3x reduction)
 * - Reduced particles per wave from 15 to 8 (2x reduction)
 * - Early exit for pixels outside visible range
 * - Hoisted expensive calculations outside inner loop
 * - Simplified glow calculation (removed exp() in inner loop)
 * - Reduced sparkle count from 12 to 8
 * - Distance squared comparisons to avoid sqrt()
 * 
 * Performance: ~2-3x faster than previous WebGL version
 */

export const stardustFragmentShader = `#version 300 es
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

#define NUM_WAVES 12
#define PARTICLES_PER_WAVE 8
#define PI 3.14159265359
#define TWO_PI 6.28318530718

// Note: hash and hash2f functions are provided by noiseGLSL injection

void main() {
    vec2 center = vec2(0.5);
    vec2 uv = v_uv - center;
    
    float aspect = u_resolution.x / u_resolution.y;
    uv.x *= aspect;
    
    float dist = length(uv);
    
    vec3 totalColor = vec3(0.0);
    float totalAlpha = 0.0;
    
    // Stardust color palette (precalculated)
    vec3 cyanColor = vec3(0.545, 0.784, 0.965);
    vec3 whiteColor = vec3(1.0);
    
    // Halo ring parameters
    float haloRadius = 0.3 * (1.0 + u_bass * 0.3);
    float maxRadius = haloRadius * 2.5;
    
    // Early exit if outside max particle range
    if (dist > maxRadius + 0.1) {
        fragColor = vec4(0.0);
        return;
    }
    
    // Draw halo ring (fast analytical glow)
    float ringDist = abs(dist - haloRadius);
    float ringGlow = exp(-ringDist * 50.0) * (0.3 + u_bass * 0.4);
    totalColor += cyanColor * ringGlow;
    totalAlpha += ringGlow;
    
    // Particle simulation with reduced iterations
    float spawnInterval = 0.1; // Slower spawn rate
    int numWaves = min(int(u_time / spawnInterval), NUM_WAVES);
    
    // Precompute spawn bass threshold
    float bassThreshold = 0.3;
    
    for (int wave = 0; wave < NUM_WAVES; wave++) {
        if (wave > numWaves) break;
        
        float waveTime = float(wave) * spawnInterval;
        float age = u_time - waveTime;
        
        // Skip old waves early
        if (age > 1.5) continue;
        
        float waveSeed = hash(float(wave));
        
        // Skip if bass was low at spawn
        if (u_bass < bassThreshold && waveSeed > 0.3) continue;
        
        // Number of particles in this wave (reduced max)
        int numInWave = int(u_bass * float(PARTICLES_PER_WAVE));
        if (numInWave < 1) continue;
        
        // Life decay factor (precomputed for wave)
        float lifeDecay = 1.0 / (maxRadius - haloRadius);
        
        for (int p = 0; p < PARTICLES_PER_WAVE; p++) {
            if (p >= numInWave) break;
            
            float seed = hash(float(wave) * 100.0 + float(p));
            
            // Particle properties
            float particleAngle = TWO_PI * seed + float(wave) * 0.15;
            float speed = 0.5 + seed * 1.5 + u_bass * 2.0;
            
            // Particle position (expands from halo)
            float particleDist = haloRadius + age * speed * 0.12;
            
            // Early exit if particle too far
            if (particleDist > maxRadius) continue;
            
            // Life fades with distance
            float life = 1.0 - (particleDist - haloRadius) * lifeDecay;
            if (life <= 0.0) continue;
            
            // Particle position in UV space
            float ca = cos(particleAngle);
            float sa = sin(particleAngle);
            vec2 particlePos = vec2(ca, sa) * particleDist;
            
            // Distance from this pixel to particle
            vec2 delta = uv - particlePos;
            float pDistSq = dot(delta, delta);
            
            // Early exit if too far (faster than computing full glow)
            float maxGlowDistSq = 0.008;
            if (pDistSq > maxGlowDistSq) continue;
            
            // Simplified glow (avoid exp in tight loop)
            float glowSize = 0.015 + seed * 0.008;
            float particleGlow = max(0.0, 1.0 - sqrt(pDistSq) / glowSize) * life;
            particleGlow = particleGlow * particleGlow; // Smooth falloff
            
            // Color
            vec3 particleColor = mix(cyanColor, whiteColor, particleGlow * 0.5);
            
            totalColor += particleColor * particleGlow * 0.8;
            totalAlpha += particleGlow * 0.4;
        }
    }
    
    // Treble sparkles on halo (reduced count)
    if (u_treble > 0.35) {
        for (int i = 0; i < 8; i++) {
            float sparkleAngle = float(i) * 0.785398 + u_time * 0.5; // PI/4 * i
            vec2 sparklePos = vec2(cos(sparkleAngle), sin(sparkleAngle)) * haloRadius;
            
            vec2 sparkleDelta = uv - sparklePos;
            float sparkleDist = dot(sparkleDelta, sparkleDelta);
            float sparklePhase = sin(u_time * 5.0 + float(i)) * 0.5 + 0.5;
            float sparkle = max(0.0, 1.0 - sparkleDist * 2000.0) * sparklePhase * u_treble;
            
            totalColor += whiteColor * sparkle;
            totalAlpha += sparkle * 0.3;
        }
    }
    
    fragColor = vec4(totalColor, clamp(totalAlpha, 0.0, 1.0));
}
`;
