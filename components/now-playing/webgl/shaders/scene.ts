/** Shared coordinates, audio response, and glow for the scene shaders. */
export const sceneFragmentHeader = `#version 300 es
precision highp float;

uniform vec2 u_resolution;
uniform float u_time;
uniform sampler2D u_waveformTexture;
uniform sampler2D u_frequencyTexture;
uniform float u_bass;
uniform float u_mid;
uniform float u_treble;

in vec2 v_uv;
out vec4 fragColor;

const float TAU = 6.2831853;

vec2 scenePosition() {
    return (v_uv - 0.5) * u_resolution / min(u_resolution.x, u_resolution.y);
}

float energy(float value) {
    return sqrt(clamp(value, 0.0, 1.0));
}

float band(float x) {
    return energy(getFrequency(u_frequencyTexture, clamp(x, 0.0, 1.0)));
}

float wave(float x) {
    return getWaveform(u_waveformTexture, clamp(x, 0.0, 1.0));
}

float filament(float distanceToLine, float width) {
    float d = abs(distanceToLine) / width;
    return exp(-d * d) + 0.45 * exp(-d * 0.23);
}

float starGrid(vec2 p, float density, float speed) {
    vec2 grid = p * density;
    vec2 cell = floor(grid);
    vec2 seed = hash2(cell);
    vec2 offset = fract(grid) - mix(vec2(0.14), vec2(0.86), seed);
    float blink = 0.5 + 0.5 * sin(u_time * speed + seed.x * TAU);
    return exp(-dot(offset, offset) * 250.0) * blink * step(0.43, seed.y);
}

vec4 sceneColor(vec3 light) {
    float peak = max(max(light.r, light.g), light.b);
    // The fullscreen quad uses source-alpha blending. Keep dark areas transparent.
    float alpha = sqrt(clamp(peak, 0.0, 1.0));
    return vec4(clamp(light / max(alpha, 0.001), 0.0, 1.0), alpha);
}
`;
