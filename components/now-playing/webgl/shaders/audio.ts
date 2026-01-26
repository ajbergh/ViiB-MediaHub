/**
 * GLSL Utility Functions - Audio
 * 
 * Helper functions for accessing audio texture data in shaders.
 */

export const audioGLSL = `
//
// Audio Texture Utilities
//

// Sample frequency at normalized position (0-1)
float getFrequency(sampler2D audioTex, float x) {
    return texture(audioTex, vec2(x, 0.5)).r;
}

// Sample waveform at normalized position (0-1)
// Returns value centered at 0 (-1 to 1 range)
float getWaveform(sampler2D waveformTex, float x) {
    return texture(waveformTex, vec2(x, 0.5)).r * 2.0 - 1.0;
}

// Sample waveform raw (0-1 range, 0.5 = center)
float getWaveformRaw(sampler2D waveformTex, float x) {
    return texture(waveformTex, vec2(x, 0.5)).r;
}

// Get average frequency in a range
float getFrequencyRange(sampler2D audioTex, float start, float end, int samples) {
    float sum = 0.0;
    float step = (end - start) / float(samples);
    
    for (int i = 0; i < 32; i++) {
        if (i >= samples) break;
        sum += getFrequency(audioTex, start + float(i) * step);
    }
    
    return sum / float(samples);
}

// Get bass energy (low frequencies)
float getBass(sampler2D audioTex) {
    return getFrequencyRange(audioTex, 0.0, 0.12, 8);
}

// Get mid energy (mid frequencies)
float getMid(sampler2D audioTex) {
    return getFrequencyRange(audioTex, 0.12, 0.58, 12);
}

// Get treble energy (high frequencies)
float getTreble(sampler2D audioTex) {
    return getFrequencyRange(audioTex, 0.58, 1.0, 10);
}

// Get all energy bands at once (more efficient)
vec3 getEnergyBands(sampler2D audioTex) {
    float bass = 0.0;
    float mid = 0.0;
    float treble = 0.0;
    
    // Sample 32 points across spectrum
    for (int i = 0; i < 32; i++) {
        float x = float(i) / 32.0;
        float val = getFrequency(audioTex, x);
        
        if (i < 4) {
            bass += val;
        } else if (i < 19) {
            mid += val;
        } else {
            treble += val;
        }
    }
    
    return vec3(bass / 4.0, mid / 15.0, treble / 13.0);
}

// Beat detection helper (compare to threshold)
float detectBeat(float energy, float threshold, float sensitivity) {
    return smoothstep(threshold - sensitivity, threshold + sensitivity, energy);
}

// Frequency-based color mapping
vec3 frequencyColor(float freq, float intensity) {
    // Map frequency to hue (bass=red, mid=green, treble=blue)
    float hue = freq * 0.7; // 0 to 0.7 (red through blue)
    
    // HSV to RGB conversion
    vec3 rgb = clamp(
        abs(mod(hue * 6.0 + vec3(0.0, 4.0, 2.0), 6.0) - 3.0) - 1.0,
        0.0, 1.0
    );
    
    return mix(vec3(1.0), rgb, 1.0) * intensity;
}
`;
