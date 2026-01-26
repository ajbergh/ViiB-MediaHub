/**
 * GLSL Utility Functions - Noise
 * 
 * Common noise functions used across visualizer shaders.
 * Includes Simplex 2D, Perlin noise, and hash functions.
 */

export const noiseGLSL = `
//
// GLSL Noise Functions
//

// Simple hash function
float hash(float n) {
    return fract(sin(n) * 43758.5453123);
}

// 2D hash returning vec2
vec2 hash2(vec2 p) {
    p = vec2(dot(p, vec2(127.1, 311.7)), dot(p, vec2(269.5, 183.3)));
    return fract(sin(p) * 43758.5453);
}

// 2D hash returning float (for when a single random value is needed from 2D input)
float hash2f(vec2 p) {
    return fract(sin(dot(p, vec2(127.1, 311.7))) * 43758.5453123);
}

// Permutation polynomial
vec3 permute(vec3 x) {
    return mod(((x * 34.0) + 1.0) * x, 289.0);
}

/**
 * Simplex 2D noise
 * Returns value in range [-1, 1]
 */
float snoise(vec2 v) {
    const vec4 C = vec4(
        0.211324865405187,   // (3.0 - sqrt(3.0)) / 6.0
        0.366025403784439,   // 0.5 * (sqrt(3.0) - 1.0)
        -0.577350269189626,  // -1.0 + 2.0 * C.x
        0.024390243902439    // 1.0 / 41.0
    );
    
    // First corner
    vec2 i = floor(v + dot(v, C.yy));
    vec2 x0 = v - i + dot(i, C.xx);
    
    // Other corners
    vec2 i1 = (x0.x > x0.y) ? vec2(1.0, 0.0) : vec2(0.0, 1.0);
    vec4 x12 = x0.xyxy + C.xxzz;
    x12.xy -= i1;
    
    // Permutations
    i = mod(i, 289.0);
    vec3 p = permute(permute(i.y + vec3(0.0, i1.y, 1.0)) + i.x + vec3(0.0, i1.x, 1.0));
    
    vec3 m = max(0.5 - vec3(dot(x0, x0), dot(x12.xy, x12.xy), dot(x12.zw, x12.zw)), 0.0);
    m = m * m;
    m = m * m;
    
    // Gradients
    vec3 x = 2.0 * fract(p * C.www) - 1.0;
    vec3 h = abs(x) - 0.5;
    vec3 ox = floor(x + 0.5);
    vec3 a0 = x - ox;
    
    // Normalize gradients
    m *= 1.79284291400159 - 0.85373472095314 * (a0 * a0 + h * h);
    
    // Compute final noise value
    vec3 g;
    g.x = a0.x * x0.x + h.x * x0.y;
    g.yz = a0.yz * x12.xz + h.yz * x12.yw;
    return 130.0 * dot(m, g);
}

/**
 * Fractal Brownian Motion (layered noise)
 * @param octaves Number of noise layers
 * @param lacunarity Frequency multiplier per octave
 * @param gain Amplitude multiplier per octave
 */
float fbm(vec2 p, int octaves, float lacunarity, float gain) {
    float value = 0.0;
    float amplitude = 0.5;
    float frequency = 1.0;
    
    for (int i = 0; i < 8; i++) {
        if (i >= octaves) break;
        value += amplitude * snoise(p * frequency);
        frequency *= lacunarity;
        amplitude *= gain;
    }
    
    return value;
}

// Simplified FBM with default parameters
float fbm(vec2 p) {
    return fbm(p, 4, 2.0, 0.5);
}

/**
 * Value noise (smoother than Perlin)
 */
float vnoise(vec2 p) {
    vec2 i = floor(p);
    vec2 f = fract(p);
    
    // Hermite interpolation
    vec2 u = f * f * (3.0 - 2.0 * f);
    
    // Four corners
    float a = hash(dot(i, vec2(127.1, 311.7)));
    float b = hash(dot(i + vec2(1.0, 0.0), vec2(127.1, 311.7)));
    float c = hash(dot(i + vec2(0.0, 1.0), vec2(127.1, 311.7)));
    float d = hash(dot(i + vec2(1.0, 1.0), vec2(127.1, 311.7)));
    
    return mix(mix(a, b, u.x), mix(c, d, u.x), u.y);
}

/**
 * Voronoi / Cellular noise
 * Returns distance to nearest cell center
 */
float voronoi(vec2 p) {
    vec2 i = floor(p);
    vec2 f = fract(p);
    
    float minDist = 1.0;
    
    for (int y = -1; y <= 1; y++) {
        for (int x = -1; x <= 1; x++) {
            vec2 neighbor = vec2(float(x), float(y));
            vec2 point = hash2(i + neighbor);
            vec2 diff = neighbor + point - f;
            float dist = length(diff);
            minDist = min(minDist, dist);
        }
    }
    
    return minDist;
}
`;
