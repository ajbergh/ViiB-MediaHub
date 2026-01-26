/**
 * GLSL Utility Functions - SDF (Signed Distance Fields)
 * 
 * Distance field primitives for smooth shape rendering without geometry.
 */

export const sdfGLSL = `
//
// Signed Distance Field Primitives
//

// Circle SDF
float sdCircle(vec2 p, float r) {
    return length(p) - r;
}

// Box SDF
float sdBox(vec2 p, vec2 b) {
    vec2 d = abs(p) - b;
    return length(max(d, 0.0)) + min(max(d.x, d.y), 0.0);
}

// Rounded box SDF
float sdRoundedBox(vec2 p, vec2 b, float r) {
    vec2 d = abs(p) - b + r;
    return length(max(d, 0.0)) - r + min(max(d.x, d.y), 0.0);
}

// Line segment SDF
float sdSegment(vec2 p, vec2 a, vec2 b) {
    vec2 pa = p - a;
    vec2 ba = b - a;
    float h = clamp(dot(pa, ba) / dot(ba, ba), 0.0, 1.0);
    return length(pa - ba * h);
}

// Capsule / Stadium SDF
float sdCapsule(vec2 p, vec2 a, vec2 b, float r) {
    return sdSegment(p, a, b) - r;
}

// Arc SDF (partial ring)
float sdArc(vec2 p, float r, float thickness, float startAngle, float endAngle) {
    float angle = atan(p.y, p.x);
    if (angle < startAngle) angle += 6.283185; // 2 * PI
    
    float arcDist = abs(length(p) - r) - thickness;
    
    // Check if within arc angle
    if (angle >= startAngle && angle <= endAngle) {
        return arcDist;
    }
    
    // Distance to endpoints
    vec2 startP = vec2(cos(startAngle), sin(startAngle)) * r;
    vec2 endP = vec2(cos(endAngle), sin(endAngle)) * r;
    return min(length(p - startP), length(p - endP)) - thickness;
}

// Ring SDF
float sdRing(vec2 p, float r, float thickness) {
    return abs(length(p) - r) - thickness;
}

// Equilateral triangle SDF
float sdTriangle(vec2 p, float r) {
    const float k = sqrt(3.0);
    p.x = abs(p.x) - r;
    p.y = p.y + r / k;
    if (p.x + k * p.y > 0.0) {
        p = vec2(p.x - k * p.y, -k * p.x - p.y) / 2.0;
    }
    p.x -= clamp(p.x, -2.0 * r, 0.0);
    return -length(p) * sign(p.y);
}

// Star SDF
float sdStar(vec2 p, float r, int n, float inset) {
    float angle = atan(p.y, p.x);
    float segment = 6.283185 / float(n);
    float a = mod(angle + segment * 0.5, segment) - segment * 0.5;
    
    float outerR = r;
    float innerR = r * inset;
    
    vec2 q = vec2(cos(a), abs(sin(a)));
    vec2 tip = vec2(outerR, 0.0);
    vec2 valley = vec2(innerR * cos(segment * 0.5), innerR * sin(segment * 0.5));
    
    float d1 = sdSegment(q * length(p), tip, valley);
    return d1;
}

//
// SDF Operations
//

// Smooth union
float opSmoothUnion(float d1, float d2, float k) {
    float h = clamp(0.5 + 0.5 * (d2 - d1) / k, 0.0, 1.0);
    return mix(d2, d1, h) - k * h * (1.0 - h);
}

// Smooth subtraction
float opSmoothSubtraction(float d1, float d2, float k) {
    float h = clamp(0.5 - 0.5 * (d2 + d1) / k, 0.0, 1.0);
    return mix(d2, -d1, h) + k * h * (1.0 - h);
}

// Smooth intersection
float opSmoothIntersection(float d1, float d2, float k) {
    float h = clamp(0.5 - 0.5 * (d2 - d1) / k, 0.0, 1.0);
    return mix(d2, d1, h) + k * h * (1.0 - h);
}

// Round an SDF
float opRound(float d, float r) {
    return d - r;
}

// Annular (hollow) version
float opAnnular(float d, float r) {
    return abs(d) - r;
}

//
// SDF Rendering Utilities
//

// Convert SDF to alpha (soft edge)
float sdfAlpha(float d, float softness) {
    return 1.0 - smoothstep(0.0, softness, d);
}

// SDF glow (inverse square falloff)
float sdfGlow(float d, float intensity) {
    return intensity / (1.0 + d * d * 100.0);
}

// SDF glow with exponential falloff
float sdfGlowExp(float d, float intensity, float falloff) {
    return intensity * exp(-abs(d) * falloff);
}

// Neon-style glow (multiple layers)
vec3 sdfNeon(float d, vec3 color, float intensity) {
    float glow1 = exp(-abs(d) * 10.0) * intensity;
    float glow2 = exp(-abs(d) * 5.0) * intensity * 0.5;
    float glow3 = exp(-abs(d) * 2.0) * intensity * 0.25;
    
    vec3 core = vec3(1.0) * glow1;
    vec3 mid = color * glow2;
    vec3 outer = color * 0.5 * glow3;
    
    return core + mid + outer;
}
`;
